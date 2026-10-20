// Package auth handles GitHub credentials. Tokens stay in this package and are
// returned only to backend callers that need to make GitHub requests.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	MethodCLI = "gh"
	MethodPAT = "pat"
	service   = "pr-nanny-github"
)

var (
	ErrInvalidToken    = &Failure{kind: "auth", message: "GitHub token is invalid"}
	ErrNoCredential    = &Failure{kind: "auth", message: "GitHub credential is unavailable"}
	ErrCredentialStore = &Failure{kind: "auth", message: "secure credential storage is unavailable"}
	ErrCLIUnavailable  = &Failure{kind: "auth", message: "GitHub CLI authentication is unavailable"}
	ErrUnavailable     = &Failure{kind: "offline", message: "GitHub is temporarily unavailable"}
	ErrRateLimited     = &Failure{kind: "rate_limit", message: "GitHub rate limit reached", retry: time.Minute}
)

type Failure struct {
	kind, message string
	retry         time.Duration
}

func (e *Failure) Error() string             { return e.message }
func (e *Failure) Kind() string              { return e.kind }
func (e *Failure) RetryDelay() time.Duration { return e.retry }

type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string             { return ErrRateLimited.Error() }
func (e *RateLimitError) Is(target error) bool      { return target == ErrRateLimited }
func (e *RateLimitError) Kind() string              { return "rate_limit" }
func (e *RateLimitError) RetryDelay() time.Duration { return e.RetryAfter }

// Store is the native credential-store boundary. Implementations must never
// persist secrets outside the operating system credential vault.
type Store interface {
	Get(service, account string) (string, error)
	Set(service, account, secret string) error
	Delete(service, account string) error
}

// Validator verifies a GitHub token and returns the authenticated login.
type Validator interface {
	Validate(context.Context, string) (string, error)
}

// CLI obtains the currently selected github.com gh token on demand.
type CLI interface {
	Token(context.Context) (string, error)
}

type Manager struct {
	store     Store
	validator Validator
	cli       CLI
}

func NewManager(store Store, validator Validator, cli CLI) *Manager {
	return &Manager{store: store, validator: validator, cli: cli}
}

// ConnectPAT validates before replacing the saved credential. The token is
// never returned or retained by Manager after the credential-store call.
func (m *Manager) ConnectPAT(ctx context.Context, token string) (string, error) {
	if m == nil || m.store == nil || m.validator == nil || !validTokenShape(token) {
		return "", ErrInvalidToken
	}
	login, err := m.validator.Validate(ctx, token)
	if err != nil {
		return "", validationError(err)
	}
	if !validLogin(login) {
		return "", ErrInvalidToken
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := m.store.Set(service, login, token); err != nil {
		return "", ErrCredentialStore
	}
	return login, nil
}

// ConnectCLI confirms that gh has an authenticated github.com account. Its
// token is fetched only for the validation request and is never persisted.
func (m *Manager) ConnectCLI(ctx context.Context) (string, error) {
	if m == nil || m.cli == nil || m.validator == nil {
		return "", ErrCLIUnavailable
	}
	token, err := m.cli.Token(ctx)
	if err != nil || !validTokenShape(token) {
		return "", ErrCLIUnavailable
	}
	login, err := m.validator.Validate(ctx, token)
	if err != nil {
		return "", validationError(err)
	}
	if !validLogin(login) {
		return "", ErrInvalidToken
	}
	return login, nil
}

// Load returns the credential for server-side GitHub requests. CLI credentials
// are read from gh each time, keeping PR Nanny from holding a second copy.
func (m *Manager) Load(ctx context.Context, method, account string) (string, string, error) {
	if m == nil || m.validator == nil {
		return "", "", ErrNoCredential
	}
	if !validLogin(account) {
		return "", "", ErrNoCredential
	}
	var token string
	var err error
	switch method {
	case MethodCLI:
		if m.cli == nil {
			return "", "", ErrCLIUnavailable
		}
		token, err = m.cli.Token(ctx)
	case MethodPAT:
		if m.store == nil {
			return "", "", ErrCredentialStore
		}
		token, err = m.store.Get(service, account)
	default:
		return "", "", ErrNoCredential
	}
	if err != nil || !validTokenShape(token) {
		return "", "", ErrNoCredential
	}
	login, err := m.validator.Validate(ctx, token)
	if err != nil || !validLogin(login) || !strings.EqualFold(login, account) {
		if err != nil {
			return "", "", validationError(err)
		}
		return "", "", ErrInvalidToken
	}
	return token, login, nil
}

func (m *Manager) ForgetPAT(account string) error {
	if m == nil || m.store == nil {
		return ErrCredentialStore
	}
	if !validLogin(account) {
		return ErrNoCredential
	}
	if err := m.store.Delete(service, account); err != nil {
		return ErrCredentialStore
	}
	return nil
}

func validTokenShape(token string) bool {
	if len(token) < 20 || len(token) > 300 || strings.TrimSpace(token) != token {
		return false
	}
	for _, r := range token {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

func validLogin(login string) bool {
	if login == "" || len(login) > 39 {
		return false
	}
	for i, r := range login {
		validCharacter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-'
		if !validCharacter || i == 0 && r == '-' {
			return false
		}
	}
	return !strings.HasSuffix(login, "-") && !strings.Contains(login, "--")
}

// HTTPValidator uses GitHub.com's authenticated user endpoint. A fixed URL
// prevents token exfiltration through caller-controlled API endpoints.
type HTTPValidator struct{ Client *http.Client }

func (v HTTPValidator) Validate(ctx context.Context, token string) (string, error) {
	if !validTokenShape(token) {
		return "", ErrInvalidToken
	}
	client := v.Client
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	client = withoutRedirects(client)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return "", ErrInvalidToken
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := client.Do(req)
	if err != nil {
		return "", ErrUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden && authIsRateLimit(resp) {
		return "", authRateError(resp)
	}
	if resp.StatusCode >= 500 {
		return "", ErrUnavailable
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096)) // Best-effort drain after a failed request.
		return "", ErrInvalidToken
	}
	var user struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&user); err != nil || !validLogin(user.Login) {
		return "", ErrInvalidToken
	}
	return user.Login, nil
}

// GHCLI uses only the standard GitHub host and returns the token in memory to
// its caller. stderr is discarded because command diagnostics may include
// sensitive environment details; all errors are replaced with safe sentinels.
type GHCLI struct {
	LookPath func(string) (string, error)
	Run      func(context.Context, string, ...string) ([]byte, error)
}

func (g GHCLI) Token(ctx context.Context) (string, error) {
	path := exec.LookPath
	if g.LookPath != nil {
		path = g.LookPath
	}
	bin, err := path("gh")
	if err != nil {
		for _, candidate := range []string{"/opt/homebrew/bin/gh", "/usr/local/bin/gh", "/usr/bin/gh"} {
			if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
				bin = candidate
				break
			}
		}
		if bin == "" {
			return "", ErrCLIUnavailable
		}
	}
	run := g.Run
	if run == nil {
		run = func(ctx context.Context, bin string, args ...string) ([]byte, error) {
			// The renderer cannot choose the executable or arguments; the caller below is fixed.
			cmd := exec.CommandContext(ctx, bin, "auth", "token", "--hostname", "github.com") // #nosec G204 -- backend-resolved gh executable with constant argv; no shell or renderer input.
			var stdout limitedBuffer
			stdout.limit = 4096
			cmd.Stdout = &stdout
			cmd.Stderr = io.Discard
			if err := cmd.Run(); err != nil {
				return nil, err
			}
			return bytes.TrimSpace(stdout.Bytes()), nil
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out, err := run(ctx, bin, "auth", "token", "--hostname", "github.com")
	if err != nil {
		return "", ErrCLIUnavailable
	}
	token := strings.TrimSpace(string(out))
	if !validTokenShape(token) {
		return "", ErrCLIUnavailable
	}
	return token, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func withoutRedirects(source *http.Client) *http.Client {
	client := *source
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client
}

func authRateError(resp *http.Response) error {
	delay := time.Minute
	if value := resp.Header.Get("Retry-After"); value != "" {
		if n, err := strconv.Atoi(value); err == nil && n >= 0 && n <= 86400 {
			delay = time.Duration(n) * time.Second
		}
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		if seconds, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			if d := time.Until(time.Unix(seconds, 0)); d > delay && d <= 24*time.Hour {
				delay = d
			}
		}
	}
	return &RateLimitError{RetryAfter: delay}
}

func authIsRateLimit(resp *http.Response) bool {
	if resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := strings.ToLower(string(body))
	return strings.Contains(message, "secondary rate limit") || strings.Contains(message, "rate limit exceeded")
}

func validationError(err error) error {
	var failure interface {
		Kind() string
		RetryDelay() time.Duration
	}
	if errors.As(err, &failure) {
		switch failure.Kind() {
		case "rate_limit":
			return &RateLimitError{RetryAfter: failure.RetryDelay()}
		case "offline":
			return ErrUnavailable
		}
	}
	return ErrInvalidToken
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() >= b.limit {
		return n, nil
	}
	if len(p) > b.limit-b.Len() {
		p = p[:b.limit-b.Len()]
	}
	_, err := b.Buffer.Write(p)
	return n, err
}
