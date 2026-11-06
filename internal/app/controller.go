// Package app implements the narrow, non-secret UI operations independently of
// any user interface. Opening URLs and showing windows belong to the UI process.
package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/alexcatdad/pr-nanny/internal/auth"
	"github.com/alexcatdad/pr-nanny/internal/core"
	"github.com/alexcatdad/pr-nanny/internal/model"
	"github.com/alexcatdad/pr-nanny/internal/storage"
)

var (
	ErrSettings        = errors.New("Unable to save settings. Check local storage and try again.")
	ErrAuthentication  = errors.New("Unable to connect to GitHub. Check authentication in Settings.")
	ErrUnavailable     = errors.New("That pull request is no longer available. Refresh and try again.")
	ErrAutostart       = errors.New("Unable to change launch at login. Check your operating system settings.")
	ErrCredentialStore = errors.New("Native credential storage is unavailable. Unlock your credential store and try again.")
	ErrNoLegacyToken   = errors.New("No token saved by an earlier PR Nanny version was found.")
)

type Auth interface {
	ConnectPAT(context.Context, string) (string, error)
	ConnectCLI(context.Context) (string, error)
	Load(context.Context, string, string) (string, string, error)
	ForgetPAT(string) error
	Provide(account, token string) error
	LegacyToken(account string) (string, error)
	DeleteLegacy(account string) error
}
type Preferences interface {
	core.Persistence
	Settings() (model.Settings, error)
	SaveSettings(model.Settings) error
}

// Autostart applies launch-at-login in UIs that delegate it to Go. A nil
// Autostart only persists the preference; the UI process then owns it.
type Autostart interface {
	SetAutostart(bool) error
}
type Controller struct {
	mu         sync.Mutex
	operations sync.Mutex
	settings   model.Settings
	store      Preferences
	auth       Auth
	monitor    *core.Monitor
	autostart  Autostart
	ctx        context.Context
	cancel     context.CancelFunc
	warning    string
}

func New(store Preferences, credentials Auth, fetcher core.Fetcher, notifier core.Notifier, autostart Autostart) (*Controller, error) {
	settings, err := store.Settings()
	if err != nil {
		return nil, err
	}
	if !ValidSettings(settings) {
		return nil, storage.ErrCorrupt
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Controller{settings: settings, store: store, auth: credentials, autostart: autostart, ctx: ctx, cancel: cancel}
	monitor, err := core.New(fetcher, c.credentials, store, notifier, settings)
	if err != nil {
		cancel()
		return nil, err
	}
	c.monitor = monitor
	return c, nil
}
func ValidSettings(s model.Settings) bool {
	return s.Version == 1 && (s.AuthMethod == "gh" || s.AuthMethod == "pat") && (s.Layout == "all" || s.Layout == "tabs") && (s.IntervalSeconds == 30 || s.IntervalSeconds == 60 || s.IntervalSeconds == 120 || s.IntervalSeconds == 300) && len(s.Account) <= 39
}
func (c *Controller) current() model.Settings { c.mu.Lock(); defer c.mu.Unlock(); return c.settings }
func (c *Controller) credentials(ctx context.Context) (string, string, error) {
	s := c.current()
	token, account, err := c.auth.Load(ctx, s.AuthMethod, s.Account)
	if err != nil {
		return "", "", classifyAuth(err)
	}
	return token, account, nil
}
func (c *Controller) Run() {
	// Discover the existing CLI account on first launch without a login flow.
	if s := c.current(); s.Account == "" && s.AuthMethod == "gh" {
		_, _ = c.ConnectCLI()
	}
	c.monitor.Run(c.ctx)
}
func (c *Controller) Stop() { c.cancel() }
func (c *Controller) GetSnapshot() (model.Snapshot, error) {
	s := c.monitor.Snapshot()
	c.mu.Lock()
	warning := c.warning
	c.mu.Unlock()
	if warning != "" {
		if s.Message != "" {
			s.Message += ". "
		}
		s.Message += warning
	}
	return s, nil
}
func (c *Controller) SaveSettings(next model.Settings) (model.Snapshot, error) {
	c.operations.Lock()
	defer c.operations.Unlock()
	old := c.current()
	if !ValidSettings(next) || next.AuthMethod != old.AuthMethod || next.Account != old.Account {
		return model.Snapshot{}, ErrSettings
	}
	if next.LaunchAtLogin != old.LaunchAtLogin && c.autostart != nil {
		if err := c.autostart.SetAutostart(next.LaunchAtLogin); err != nil {
			return model.Snapshot{}, ErrAutostart
		}
	}
	if err := c.commitSettings(next); err != nil {
		if next.LaunchAtLogin != old.LaunchAtLogin && c.autostart != nil {
			_ = c.autostart.SetAutostart(old.LaunchAtLogin)
		}
		return model.Snapshot{}, err
	}
	return c.GetSnapshot()
}
func (c *Controller) commitSettings(next model.Settings) error {
	old := c.current()
	if err := c.store.SaveSettings(next); err != nil {
		return ErrSettings
	}
	c.mu.Lock()
	c.settings = next
	c.mu.Unlock()
	if err := c.monitor.SetSettings(next); err != nil {
		c.mu.Lock()
		c.settings = old
		c.mu.Unlock()
		_ = c.store.SaveSettings(old)
		return ErrSettings
	}
	return nil
}
func (c *Controller) ConnectPAT(token string) (model.Snapshot, error) {
	c.operations.Lock()
	defer c.operations.Unlock()
	previous := c.current()
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	account, err := c.auth.ConnectPAT(ctx, token)
	if err != nil {
		return model.Snapshot{}, safeAuthError(err)
	}
	snapshot, err := c.connect("pat", account)
	if err != nil && (previous.AuthMethod != "pat" || previous.Account != account) {
		// A newly staged credential must not survive a failed account selection.
		_ = c.auth.ForgetPAT(account)
	}
	return snapshot, err
}
func (c *Controller) ConnectCLI() (model.Snapshot, error) {
	c.operations.Lock()
	defer c.operations.Unlock()
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	account, err := c.auth.ConnectCLI(ctx)
	if err != nil {
		return model.Snapshot{}, safeAuthError(err)
	}
	return c.connect("gh", account)
}
func (c *Controller) connect(method, account string) (model.Snapshot, error) {
	old := c.current()
	next := old
	next.AuthMethod = method
	next.Account = account
	if err := c.commitSettings(next); err != nil {
		return model.Snapshot{}, err
	}
	if old.AuthMethod == "pat" && (method != "pat" || old.Account != account) {
		if c.auth.ForgetPAT(old.Account) != nil {
			c.SetWarning("The previous token could not be removed from the native credential store. Remove the old PR Nanny entry there.")
		}
	}
	// A replacement token for the same account also needs an immediate fetch.
	go func() { _ = c.monitor.Refresh(c.ctx) }()
	return c.GetSnapshot()
}

// ProvideToken loads the PAT for the configured account from the app's
// keychain into memory and starts a fetch. Only the configured PAT account
// is accepted.
func (c *Controller) ProvideToken(account, token string) (model.Snapshot, error) {
	s := c.current()
	if s.AuthMethod != auth.MethodPAT || s.Account == "" || !strings.EqualFold(s.Account, account) {
		return model.Snapshot{}, ErrAuthentication
	}
	if err := c.auth.Provide(s.Account, token); err != nil {
		return model.Snapshot{}, safeAuthError(err)
	}
	go func() { _ = c.monitor.Refresh(c.ctx) }()
	return c.GetSnapshot()
}

// LegacyToken returns the configured account's PAT from the login-keychain
// item PR Nanny 0.1.x wrote, and loads it. The app stores it in its own
// keychain item and then calls DeleteLegacyToken.
func (c *Controller) LegacyToken() (string, string, error) {
	s := c.current()
	if s.AuthMethod != auth.MethodPAT || s.Account == "" {
		return "", "", ErrNoLegacyToken
	}
	token, err := c.auth.LegacyToken(s.Account)
	if errors.Is(err, auth.ErrNoCredential) {
		return "", "", ErrNoLegacyToken
	}
	if err != nil {
		return "", "", safeAuthError(err)
	}
	go func() { _ = c.monitor.Refresh(c.ctx) }()
	return s.Account, token, nil
}

// DeleteLegacyToken removes the configured account's PR Nanny 0.1.x item.
func (c *Controller) DeleteLegacyToken() error {
	s := c.current()
	if s.Account == "" {
		return nil
	}
	if c.auth.DeleteLegacy(s.Account) != nil {
		return ErrCredentialStore
	}
	return nil
}

func (c *Controller) SetBell(id string, enabled bool) (model.Snapshot, error) {
	if len(id) == 0 || len(id) > 256 {
		return model.Snapshot{}, ErrUnavailable
	}
	if err := c.monitor.SetBell(id, enabled); err != nil {
		return model.Snapshot{}, ErrSettings
	}
	return c.GetSnapshot()
}
func (c *Controller) SetLayout(layout string) (model.Snapshot, error) {
	c.operations.Lock()
	defer c.operations.Unlock()
	if layout != "all" && layout != "tabs" {
		return model.Snapshot{}, ErrSettings
	}
	s := c.current()
	s.Layout = layout
	if err := c.commitSettings(s); err != nil {
		return model.Snapshot{}, err
	}
	return c.GetSnapshot()
}
func (c *Controller) Refresh() error {
	// Fetches are asynchronous; the UI reads refresh/error state through snapshots.
	go func() { _ = c.monitor.Refresh(c.ctx) }()
	return nil
}

// ResolvePR returns the canonical GitHub.com URL of a currently listed pull
// request. Callers open it themselves; unknown IDs and non-canonical URLs fail.
func (c *Controller) ResolvePR(id string) (string, error) {
	snapshot, _ := c.GetSnapshot()
	for _, pr := range snapshot.PullRequests {
		if pr.ID == id {
			if !ValidPRURL(pr.URL) {
				return "", ErrUnavailable
			}
			return pr.URL, nil
		}
	}
	return "", ErrUnavailable
}

// NotificationAllowed applies the user's gating at delivery time: the global
// switch, then the per-event preference, then the PR's bell. Review requests
// bypass the bell; PRs no longer listed never notify.
func (c *Controller) NotificationAllowed(id string, event core.Event) bool {
	snapshot, _ := c.GetSnapshot()
	s := snapshot.Settings
	if !s.Notifications {
		return false
	}
	known := false
	for _, pr := range snapshot.PullRequests {
		if pr.ID == id {
			known = event == core.EventReviewRequested || pr.Bell
			break
		}
	}
	if !known {
		return false
	}
	switch event {
	case core.EventCI:
		return s.Events.CI
	case core.EventApproval:
		return s.Events.Approval
	case core.EventReviewRequested:
		return s.Events.ReviewRequested
	case core.EventMerged:
		return s.Events.Merged
	case core.EventClosed:
		return s.Events.Closed
	}
	return false
}
func (c *Controller) ApplyStartup() {
	if c.autostart == nil {
		return
	}
	if err := c.autostart.SetAutostart(c.current().LaunchAtLogin); err != nil {
		c.mu.Lock()
		c.warning = "Launch at login could not be configured; check Settings"
		c.mu.Unlock()
	}
}

type authFailure struct{ kind string }

func (e authFailure) Error() string             { return "GitHub authentication needs attention" }
func (e authFailure) Kind() string              { return e.kind }
func (e authFailure) RetryDelay() time.Duration { return 0 }
func classifyAuth(err error) error {
	var classified core.ClassifiedError
	if errors.As(err, &classified) {
		return err
	}
	return authFailure{kind: "auth"}
}
func safeAuthError(err error) error {
	if errors.Is(err, auth.ErrCredentialStore) {
		return ErrCredentialStore
	}
	return ErrAuthentication
}

// SetWarning adds a safe local operational warning to the view.
func (c *Controller) SetWarning(message string) { c.mu.Lock(); c.warning = message; c.mu.Unlock() }
