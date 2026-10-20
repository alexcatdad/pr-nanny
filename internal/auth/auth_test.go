package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type memoryStore struct {
	value   string
	account string
	setErr  error
	sets    int
}

func (s *memoryStore) Get(_, account string) (string, error) {
	if s.value == "" || account != s.account {
		return "", ErrNoCredential
	}
	return s.value, nil
}
func (s *memoryStore) Set(_, account, value string) error {
	s.sets++
	if s.setErr != nil {
		return s.setErr
	}
	s.value = value
	s.account = account
	return nil
}
func (s *memoryStore) Delete(_, account string) error {
	if s.account == account {
		s.value = ""
		s.account = ""
	}
	return nil
}

type validatorFunc func(context.Context, string) (string, error)

func (f validatorFunc) Validate(ctx context.Context, token string) (string, error) {
	return f(ctx, token)
}

type cliFunc func(context.Context) (string, error)

func (f cliFunc) Token(ctx context.Context) (string, error) { return f(ctx) }

func TestConnectPATValidatesBeforeSecureStorage(t *testing.T) {
	store := &memoryStore{value: "old-token"}
	marker := errors.New("bad")
	manager := NewManager(store, validatorFunc(func(_ context.Context, token string) (string, error) {
		if token != "ghp_123456789012345678901234567890" {
			t.Fatal("validator did not receive submitted token")
		}
		return "", marker
	}), nil)
	_, err := manager.ConnectPAT(context.Background(), "ghp_123456789012345678901234567890")
	if !errors.Is(err, ErrInvalidToken) || store.sets != 0 || store.value != "old-token" {
		t.Fatalf("bad token replaced old credential: err=%v store=%+v", err, store)
	}
	if strings.Contains(err.Error(), "ghp_") || strings.Contains(err.Error(), "bad") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestConnectPATStoresOnlyAfterValidationAndNeverReturnsToken(t *testing.T) {
	store := &memoryStore{}
	manager := NewManager(store, validatorFunc(func(_ context.Context, token string) (string, error) { return "octocat", nil }), nil)
	login, err := manager.ConnectPAT(context.Background(), "ghp_123456789012345678901234567890")
	if err != nil || login != "octocat" {
		t.Fatalf("ConnectPAT() = %q, %v", login, err)
	}
	if store.sets != 1 || store.value == "" {
		t.Fatalf("credential was not stored: %+v", store)
	}
	if store.account != "octocat" {
		t.Fatalf("PAT was not account scoped: %q", store.account)
	}
	if store.value == login {
		t.Fatal("login unexpectedly contained the PAT")
	}
}

func TestLoadSelectsAccountCredentialAndPreservesSafeValidationKinds(t *testing.T) {
	store := &memoryStore{value: "ghp_123456789012345678901234567890", account: "octocat"}
	manager := NewManager(store, validatorFunc(func(context.Context, string) (string, error) { return "octocat", nil }), nil)
	_, _, err := manager.Load(context.Background(), MethodPAT, "another-user")
	if !errors.Is(err, ErrNoCredential) {
		t.Fatalf("wrong account credential loaded: %v", err)
	}
	token, login, err := manager.Load(context.Background(), MethodPAT, "octocat")
	if err != nil || token != store.value || login != "octocat" {
		t.Fatalf("Load() = %q %q %v", token, login, err)
	}
	manager = NewManager(store, validatorFunc(func(context.Context, string) (string, error) { return "", ErrUnavailable }), nil)
	_, _, err = manager.Load(context.Background(), MethodPAT, "octocat")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("network error was misreported as invalid token: %v", err)
	}
}

func TestConnectPATFailsClosedWhenVaultFails(t *testing.T) {
	store := &memoryStore{setErr: errors.New("sensitive backend diagnostics")}
	manager := NewManager(store, validatorFunc(func(context.Context, string) (string, error) { return "octocat", nil }), nil)
	_, err := manager.ConnectPAT(context.Background(), "ghp_123456789012345678901234567890")
	if !errors.Is(err, ErrCredentialStore) {
		t.Fatalf("expected safe vault error, got %v", err)
	}
	if strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("vault details leaked: %v", err)
	}
}

func TestCLIIsUsedOnDemandAndNeverCopiedToStore(t *testing.T) {
	store := &memoryStore{}
	cli := cliFunc(func(context.Context) (string, error) { return "gho_123456789012345678901234567890", nil })
	manager := NewManager(store, validatorFunc(func(_ context.Context, token string) (string, error) {
		if token != "gho_123456789012345678901234567890" {
			t.Fatal("unexpected CLI token")
		}
		return "octocat", nil
	}), cli)
	login, err := manager.ConnectCLI(context.Background())
	if err != nil || login != "octocat" {
		t.Fatalf("ConnectCLI() = %q, %v", login, err)
	}
	if store.sets != 0 || store.value != "" {
		t.Fatal("CLI token was persisted")
	}
}

func TestGHCLIRunsForGitHubHostAndDiscardsDiagnostics(t *testing.T) {
	var got []string
	cli := GHCLI{
		LookPath: func(name string) (string, error) {
			if name != "gh" {
				t.Fatalf("looked up %q", name)
			}
			return "/test/gh", nil
		},
		Run: func(_ context.Context, path string, args ...string) ([]byte, error) {
			got = append([]string{path}, args...)
			return []byte("ghp_123456789012345678901234567890\n"), nil
		},
	}
	token, err := cli.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "ghp_123456789012345678901234567890" {
		t.Fatal("token was not trimmed")
	}
	if strings.Join(got, " ") != "/test/gh auth token --hostname github.com" {
		t.Fatalf("unexpected command: %v", got)
	}
}

func TestHTTPValidatorUsesGitHubAPIAndDoesNotExposeBodyInErrors(t *testing.T) {
	var gotURL, authorization string
	client := &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		gotURL, authorization = req.URL.String(), req.Header.Get("Authorization")
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"login":"octocat"}`)), Header: make(http.Header)}, nil
	})}
	login, err := (HTTPValidator{Client: client}).Validate(context.Background(), "ghp_123456789012345678901234567890")
	if err != nil || login != "octocat" {
		t.Fatalf("Validate() = %q, %v", login, err)
	}
	if gotURL != "https://api.github.com/user" || authorization != "Bearer ghp_123456789012345678901234567890" {
		t.Fatalf("unexpected validation request %q %q", gotURL, authorization)
	}
}

func TestHTTPValidatorPreservesRateLimitAndDoesNotExposeBody(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		h := make(http.Header)
		h.Set("Retry-After", "45")
		return &http.Response{StatusCode: http.StatusTooManyRequests, Header: h, Body: io.NopCloser(strings.NewReader("sensitive body"))}, nil
	})}
	_, err := (HTTPValidator{Client: client}).Validate(context.Background(), "ghp_123456789012345678901234567890")
	var rate *RateLimitError
	if !errors.As(err, &rate) || rate.RetryAfter != 45*time.Second || calls != 1 || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("rate limit was not safely preserved: %v calls=%d", err, calls)
	}
}

func TestHTTPValidatorDoesNotFollowRedirects(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		h := make(http.Header)
		h.Set("Location", "https://attacker.example/collect")
		return &http.Response{StatusCode: http.StatusFound, Header: h, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	_, err := (HTTPValidator{Client: client}).Validate(context.Background(), "ghp_123456789012345678901234567890")
	if !errors.Is(err, ErrInvalidToken) || calls != 1 {
		t.Fatalf("validator followed redirect: err=%v calls=%d", err, calls)
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
