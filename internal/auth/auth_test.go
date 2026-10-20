package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type memoryStore struct {
	value   string
	account string
	setErr  error
	delErr  error
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
	if s.delErr != nil {
		return s.delErr
	}
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

func TestGHCLIMapsCommandFailuresAndMalformedTokens(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  []byte
		err  error
	}{
		{name: "command failure", err: errors.New("private command diagnostics")},
		{name: "malformed token", out: []byte("not-a-token")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := GHCLI{
				LookPath: func(string) (string, error) { return "/test/gh", nil },
				Run:      func(context.Context, string, ...string) ([]byte, error) { return tc.out, tc.err },
			}
			_, err := cli.Token(context.Background())
			if !errors.Is(err, ErrCLIUnavailable) || strings.Contains(err.Error(), "private command diagnostics") {
				t.Fatalf("unsafe CLI error = %v", err)
			}
		})
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

func TestManagerRejectsIncompleteAndMismatchedCredentials(t *testing.T) {
	t.Run("unavailable manager dependencies", func(t *testing.T) {
		if _, _, err := (*Manager)(nil).Load(context.Background(), MethodPAT, "octocat"); !errors.Is(err, ErrNoCredential) {
			t.Fatalf("nil manager Load() error = %v", err)
		}
		if _, err := NewManager(nil, nil, nil).ConnectPAT(context.Background(), "ghp_123456789012345678901234567890"); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("missing PAT dependencies error = %v", err)
		}
		if err := NewManager(nil, nil, nil).ForgetPAT("octocat"); !errors.Is(err, ErrCredentialStore) {
			t.Fatalf("missing credential store error = %v", err)
		}
	})

	t.Run("validator account differs from selected account", func(t *testing.T) {
		store := &memoryStore{value: "ghp_123456789012345678901234567890", account: "octocat"}
		manager := NewManager(store, validatorFunc(func(context.Context, string) (string, error) { return "another-user", nil }), nil)
		if _, _, err := manager.Load(context.Background(), MethodPAT, "octocat"); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("mismatched credential error = %v", err)
		}
	})

	t.Run("forget removes account credential", func(t *testing.T) {
		store := &memoryStore{value: "ghp_123456789012345678901234567890", account: "octocat"}
		if err := NewManager(store, nil, nil).ForgetPAT("octocat"); err != nil {
			t.Fatal(err)
		}
		if store.value != "" || store.account != "" {
			t.Fatalf("credential remains after ForgetPAT: %+v", store)
		}
	})
	t.Run("forget failure is safe", func(t *testing.T) {
		store := &memoryStore{delErr: errors.New("private vault detail")}
		err := NewManager(store, nil, nil).ForgetPAT("octocat")
		if !errors.Is(err, ErrCredentialStore) || strings.Contains(err.Error(), "private vault detail") {
			t.Fatalf("unsafe credential deletion error = %v", err)
		}
	})
}

func TestLoadRejectsUnavailableCredentialSourcesAndUnknownMethod(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		store  Store
		cli    CLI
		want   error
	}{
		{name: "missing CLI", method: MethodCLI, store: &memoryStore{}, want: ErrCLIUnavailable},
		{name: "CLI command failure", method: MethodCLI, cli: cliFunc(func(context.Context) (string, error) { return "", errors.New("private CLI error") }), want: ErrNoCredential},
		{name: "missing vault", method: MethodPAT, want: ErrCredentialStore},
		{name: "unknown method", method: "unknown", store: &memoryStore{}, want: ErrNoCredential},
		{name: "missing credential", method: MethodPAT, store: &memoryStore{}, want: ErrNoCredential},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := NewManager(tc.store, validatorFunc(func(context.Context, string) (string, error) { return "octocat", nil }), tc.cli)
			_, _, err := manager.Load(context.Background(), tc.method, "octocat")
			if !errors.Is(err, tc.want) {
				t.Fatalf("Load() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestConnectCLIMapsUnavailableAndInvalidTokensToSafeErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cli       CLI
		validator Validator
		want      error
	}{
		{name: "missing CLI", validator: validatorFunc(func(context.Context, string) (string, error) { return "octocat", nil }), want: ErrCLIUnavailable},
		{name: "CLI error", cli: cliFunc(func(context.Context) (string, error) { return "", errors.New("private CLI detail") }), validator: validatorFunc(func(context.Context, string) (string, error) { return "octocat", nil }), want: ErrCLIUnavailable},
		{name: "malformed token", cli: cliFunc(func(context.Context) (string, error) { return "short", nil }), validator: validatorFunc(func(context.Context, string) (string, error) { return "octocat", nil }), want: ErrCLIUnavailable},
		{name: "invalid login", cli: cliFunc(func(context.Context) (string, error) { return "ghp_123456789012345678901234567890", nil }), validator: validatorFunc(func(context.Context, string) (string, error) { return "not/a-login", nil }), want: ErrInvalidToken},
		{name: "validator rate limit", cli: cliFunc(func(context.Context) (string, error) { return "ghp_123456789012345678901234567890", nil }), validator: validatorFunc(func(context.Context, string) (string, error) { return "", ErrRateLimited }), want: ErrRateLimited},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewManager(nil, tc.validator, tc.cli).ConnectCLI(context.Background())
			if !errors.Is(err, tc.want) {
				t.Fatalf("ConnectCLI() error = %v, want %v", err, tc.want)
			}
			if strings.Contains(err.Error(), "private CLI detail") {
				t.Fatalf("CLI diagnostics leaked: %v", err)
			}
		})
	}
}

func TestHTTPValidatorClassifiesTransportAndResponseFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
		fail   bool
	}{
		{name: "transport failure", want: ErrUnavailable, fail: true},
		{name: "server error", status: http.StatusBadGateway, body: "private body", want: ErrUnavailable},
		{name: "invalid JSON", status: http.StatusOK, body: "{", want: ErrInvalidToken},
		{name: "invalid login", status: http.StatusOK, body: `{"login":"bad/login"}`, want: ErrInvalidToken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
				if tc.fail {
					return nil, errors.New("sensitive transport error")
				}
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			_, err := (HTTPValidator{Client: client}).Validate(context.Background(), "ghp_123456789012345678901234567890")
			if !errors.Is(err, tc.want) || strings.Contains(err.Error(), "sensitive") || strings.Contains(err.Error(), "private body") {
				t.Fatalf("Validate() error = %v, want safe %v", err, tc.want)
			}
		})
	}
}

func TestAuthRateLimitClassificationAndBoundedBuffer(t *testing.T) {
	for _, tc := range []struct {
		name string
		head http.Header
		body string
		want bool
	}{
		{name: "retry header", head: http.Header{"Retry-After": []string{"5"}}, want: true},
		{name: "remaining quota", head: http.Header{"X-Ratelimit-Remaining": []string{"0"}}, want: true},
		{name: "secondary limit body", body: `{"message":"Secondary rate limit"}`, want: true},
		{name: "ordinary forbidden", body: `{"message":"permission denied"}`, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{Header: tc.head, Body: io.NopCloser(strings.NewReader(tc.body))}
			if got := authIsRateLimit(resp); got != tc.want {
				t.Fatalf("authIsRateLimit() = %t, want %t", got, tc.want)
			}
			_ = resp.Body.Close()
		})
	}

	var b limitedBuffer
	b.limit = 4
	if n, err := b.Write([]byte("abcde")); err != nil || n != 5 || b.String() != "abcd" {
		t.Fatalf("first bounded write = %q, %d, %v", b.String(), n, err)
	}
	if n, err := b.Write([]byte("f")); err != nil || n != 1 || b.String() != "abcd" {
		t.Fatalf("write after limit = %q, %d, %v", b.String(), n, err)
	}
}

func TestRateLimitErrorUsesStableClassificationAndResetHint(t *testing.T) {
	rate := &RateLimitError{RetryAfter: 45 * time.Second}
	if !errors.Is(rate, ErrRateLimited) || rate.Kind() != "rate_limit" || rate.RetryDelay() != 45*time.Second || rate.Error() != ErrRateLimited.Error() {
		t.Fatalf("rate-limit metadata = is:%t kind:%q delay:%s error:%q", errors.Is(rate, ErrRateLimited), rate.Kind(), rate.RetryDelay(), rate.Error())
	}
	header := make(http.Header)
	header.Set("Retry-After", "invalid")
	header.Set("X-RateLimit-Remaining", "0")
	header.Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(3*time.Minute).Unix(), 10))
	err := authRateError(&http.Response{Header: header})
	var reset *RateLimitError
	if !errors.As(err, &reset) || reset.RetryAfter < 2*time.Minute || reset.RetryAfter > 3*time.Minute {
		t.Fatalf("reset-based retry hint = %v", err)
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
