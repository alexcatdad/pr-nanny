// Package app implements the narrow, non-secret UI operations independently of Wails.
package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/alexcatdad/pr-nanny/internal/auth"
	"github.com/alexcatdad/pr-nanny/internal/core"
	"github.com/alexcatdad/pr-nanny/internal/model"
	"github.com/alexcatdad/pr-nanny/internal/storage"
)

var (
	ErrSettings       = errors.New("Unable to save settings. Check local storage and try again.")
	ErrAuthentication = errors.New("Unable to connect to GitHub. Check authentication in Settings.")
	ErrUnavailable    = errors.New("That pull request is no longer available. Refresh and try again.")
	ErrAutostart      = errors.New("Unable to change launch at login. Check your operating system settings.")
)

type Auth interface {
	ConnectPAT(context.Context, string) (string, error)
	ConnectCLI(context.Context) (string, error)
	Load(context.Context, string, string) (string, string, error)
	ForgetPAT(string) error
}
type Preferences interface {
	core.Persistence
	Settings() (model.Settings, error)
	SaveSettings(model.Settings) error
}
type Platform interface {
	SetAutostart(bool) error
	OpenURL(string) error
	HidePopup()
}
type Controller struct {
	mu         sync.Mutex
	operations sync.Mutex
	settings   model.Settings
	store      Preferences
	auth       Auth
	monitor    *core.Monitor
	platform   Platform
	ctx        context.Context
	cancel     context.CancelFunc
	warning    string
}

func New(store Preferences, credentials Auth, fetcher core.Fetcher, notifier core.Notifier, platform Platform) (*Controller, error) {
	settings, err := store.Settings()
	if err != nil {
		return nil, err
	}
	if !ValidSettings(settings) {
		return nil, storage.ErrCorrupt
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Controller{settings: settings, store: store, auth: credentials, platform: platform, ctx: ctx, cancel: cancel}
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
	if next.LaunchAtLogin != old.LaunchAtLogin && c.platform != nil {
		if err := c.platform.SetAutostart(next.LaunchAtLogin); err != nil {
			return model.Snapshot{}, ErrAutostart
		}
	}
	if err := c.commitSettings(next); err != nil {
		if next.LaunchAtLogin != old.LaunchAtLogin && c.platform != nil {
			_ = c.platform.SetAutostart(old.LaunchAtLogin)
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
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	account, err := c.auth.ConnectPAT(ctx, token)
	token = ""
	if err != nil {
		return model.Snapshot{}, safeAuthError(err)
	}
	return c.connect("pat", account)
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
		_ = c.auth.ForgetPAT(old.Account)
	}
	// A replacement token for the same account also needs an immediate fetch.
	go func() { _ = c.monitor.Refresh(c.ctx) }()
	return c.GetSnapshot()
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
func (c *Controller) OpenPR(id string) error {
	snapshot, _ := c.GetSnapshot()
	for _, pr := range snapshot.PullRequests {
		if pr.ID == id {
			if !ValidPRURL(pr.URL) || c.platform == nil {
				return ErrUnavailable
			}
			if err := c.platform.OpenURL(pr.URL); err != nil {
				return ErrUnavailable
			}
			return nil
		}
	}
	return ErrUnavailable
}
func (c *Controller) HidePopup() {
	if c.platform != nil {
		c.platform.HidePopup()
	}
}
func (c *Controller) ApplyStartup() {
	if c.platform == nil {
		return
	}
	if err := c.platform.SetAutostart(c.current().LaunchAtLogin); err != nil {
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
		return errors.New("Native credential storage is unavailable. Unlock your credential store and try again.")
	}
	return ErrAuthentication
}

// SetWarning adds a safe local operational warning to the view.
func (c *Controller) SetWarning(message string) { c.mu.Lock(); c.warning = message; c.mu.Unlock() }
