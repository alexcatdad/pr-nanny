package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexcatdad/pr-nanny/internal/model"
	"github.com/alexcatdad/pr-nanny/internal/storage"
)

type testStore struct {
	mu       sync.Mutex
	settings model.Settings
	state    storage.State
	failSave bool
	order    *[]string
}

func (s *testStore) Settings() (model.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings, nil
}
func (s *testStore) SaveSettings(v model.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.order != nil {
		*s.order = append(*s.order, "settings")
	}
	if s.failSave {
		return errors.New("disk failure")
	}
	s.settings = v
	return nil
}
func (s *testStore) LoadState() (storage.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, nil
}
func (s *testStore) SaveState(v storage.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = v
	return nil
}

type testAuth struct {
	mu                          sync.Mutex
	connectAccount              string
	connectErr                  error
	calls                       []string
	order                       *[]string
	loadedMethod, loadedAccount string
	forget                      []string
}

func (a *testAuth) ConnectPAT(_ context.Context, token string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, "pat:"+token)
	if a.order != nil {
		*a.order = append(*a.order, "auth")
	}
	return a.connectAccount, a.connectErr
}
func (a *testAuth) ConnectCLI(context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, "cli")
	if a.order != nil {
		*a.order = append(*a.order, "auth")
	}
	return a.connectAccount, a.connectErr
}
func (a *testAuth) Load(_ context.Context, method, account string) (string, string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.loadedMethod, a.loadedAccount = method, account
	return "synthetic-secret", account, nil
}
func (a *testAuth) ForgetPAT(account string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.forget = append(a.forget, account)
	return nil
}

type testFetcher struct {
	mu      sync.Mutex
	result  model.FetchResult
	entered chan struct{}
	release chan struct{}
	calls   int
}

func (f *testFetcher) Fetch(ctx context.Context, _, _ string, _ []string) (model.FetchResult, error) {
	f.mu.Lock()
	f.calls++
	entered, release := f.entered, f.release
	result := f.result
	f.mu.Unlock()
	if entered != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return model.FetchResult{}, ctx.Err()
		}
	}
	return result, nil
}

type testPlatform struct {
	mu        sync.Mutex
	autostart []bool
	open      []string
	openErr   error
}

func (p *testPlatform) SetAutostart(v bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.autostart = append(p.autostart, v)
	return nil
}
func (p *testPlatform) OpenURL(v string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.open = append(p.open, v)
	return p.openErr
}
func (*testPlatform) HidePopup() {}

func newController(t *testing.T, settings model.Settings, store *testStore, auth *testAuth, fetcher *testFetcher, platform *testPlatform) *Controller {
	t.Helper()
	if store == nil {
		store = &testStore{settings: settings}
	}
	if auth == nil {
		auth = &testAuth{connectAccount: "octocat"}
	}
	if fetcher == nil {
		fetcher = &testFetcher{}
	}
	c, err := New(store, auth, fetcher, nil, platform)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Stop)
	return c
}
func waitForPR(t *testing.T, c *Controller, id string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s, _ := c.GetSnapshot()
		for _, p := range s.PullRequests {
			if p.ID == id {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for fetched pull request")
}
func baseSettings() model.Settings {
	s := model.DefaultSettings()
	s.Account = "old-user"
	s.LaunchAtLogin = false
	return s
}

func TestConnectPATValidatesBeforeSavingAccountMetadata(t *testing.T) {
	order := []string{}
	s := baseSettings()
	store := &testStore{settings: s, order: &order}
	auth := &testAuth{connectAccount: "new-user", order: &order}
	c := newController(t, s, store, auth, &testFetcher{}, nil)
	if _, err := c.ConnectPAT("synthetic-token"); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(order) != "[auth settings]" {
		t.Fatalf("operation order = %v", order)
	}
	if got := c.current(); got.AuthMethod != "pat" || got.Account != "new-user" {
		t.Fatalf("settings not switched: %+v", got)
	}
}

func TestFailedSettingsSaveDoesNotSelectNewAuth(t *testing.T) {
	s := baseSettings()
	store := &testStore{settings: s, failSave: true}
	auth := &testAuth{connectAccount: "new-user"}
	c := newController(t, s, store, auth, &testFetcher{}, nil)
	if _, err := c.ConnectPAT("synthetic-token"); !errors.Is(err, ErrSettings) {
		t.Fatalf("ConnectPAT error = %v", err)
	}
	if got := c.current(); got.AuthMethod != s.AuthMethod || got.Account != s.Account {
		t.Fatalf("failed save changed selected account: %+v", got)
	}
	auth.mu.Lock()
	defer auth.mu.Unlock()
	if auth.loadedMethod != "" {
		t.Fatalf("unexpected credential load after failed commit: %s", auth.loadedMethod)
	}
}

func TestSaveSettingsCannotChangeAuthentication(t *testing.T) {
	s := baseSettings()
	c := newController(t, s, nil, nil, nil, nil)
	next := s
	next.AuthMethod = "pat"
	next.Account = "attacker"
	if _, err := c.SaveSettings(next); !errors.Is(err, ErrSettings) {
		t.Fatalf("SaveSettings error = %v", err)
	}
	if got := c.current(); got.AuthMethod != s.AuthMethod || got.Account != s.Account {
		t.Fatalf("auth changed through preferences: %+v", got)
	}
}

func TestAutostartRollsBackWhenSettingsPersistenceFails(t *testing.T) {
	s := baseSettings()
	store := &testStore{settings: s, failSave: true}
	platform := &testPlatform{}
	c := newController(t, s, store, nil, nil, platform)
	next := s
	next.LaunchAtLogin = true
	if _, err := c.SaveSettings(next); !errors.Is(err, ErrSettings) {
		t.Fatalf("SaveSettings error = %v", err)
	}
	platform.mu.Lock()
	defer platform.mu.Unlock()
	if fmt.Sprint(platform.autostart) != "[true false]" {
		t.Fatalf("autostart calls = %v", platform.autostart)
	}
	if got := c.current(); got.LaunchAtLogin != s.LaunchAtLogin {
		t.Fatalf("settings changed after failed save: %+v", got)
	}
}

func TestOpenPRRequiresKnownIDAndOpensCanonicalURL(t *testing.T) {
	s := baseSettings()
	p := model.PullRequest{ID: "PR_1", URL: "https://github.com/acme/project/pull/12", Title: "<img src=x>", CI: model.CIPassed, Authored: true}
	f := &testFetcher{result: model.FetchResult{PullRequests: []model.PullRequest{p}}}
	platform := &testPlatform{}
	c := newController(t, s, nil, nil, f, platform)
	if err := c.OpenPR(p.ID); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unknown PR error = %v", err)
	}
	if err := c.Refresh(); err != nil {
		t.Fatal(err)
	}
	waitForPR(t, c, p.ID)
	if err := c.OpenPR(p.ID); err != nil {
		t.Fatal(err)
	}
	platform.mu.Lock()
	defer platform.mu.Unlock()
	if fmt.Sprint(platform.open) != "[https://github.com/acme/project/pull/12]" {
		t.Fatalf("opened URLs = %v", platform.open)
	}
}

func TestSnapshotAndErrorsNeverExposeCredential(t *testing.T) {
	const secret = "synthetic-secret-value"
	s := baseSettings()
	c := newController(t, s, nil, &testAuth{connectErr: errors.New(secret)}, nil, nil)
	_, err := c.ConnectPAT(secret)
	if err == nil {
		t.Fatal("expected auth failure")
	}
	snapshot, _ := c.GetSnapshot()
	encoded, _ := json.Marshal(snapshot)
	if strings.Contains(err.Error(), secret) || strings.Contains(string(encoded), secret) || strings.Contains(fmt.Sprintf("%+v", snapshot.Settings), secret) {
		t.Fatal("credential appeared in controller output")
	}
}

func TestConcurrentRefreshAndSettingsSave(t *testing.T) {
	s := baseSettings()
	f := &testFetcher{entered: make(chan struct{}, 4), release: make(chan struct{})}
	c := newController(t, s, nil, nil, f, nil)
	if err := c.Refresh(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.entered:
	case <-time.After(time.Second):
		t.Fatal("fetch did not start")
	}
	next := s
	next.IntervalSeconds = 120
	done := make(chan error, 1)
	go func() { _, err := c.SaveSettings(next); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("settings save blocked behind refresh")
	}
	close(f.release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, _ := c.GetSnapshot()
		if !snap.Refreshing {
			if snap.Settings.IntervalSeconds != 120 {
				t.Fatalf("settings not updated: %+v", snap.Settings)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("refresh did not finish")
}

func TestRunStopsOnShutdown(t *testing.T) {
	s := baseSettings()
	f := &testFetcher{entered: make(chan struct{}, 1), release: make(chan struct{})}
	c := newController(t, s, nil, nil, f, nil)
	done := make(chan struct{})
	go func() { c.Run(); close(done) }()
	select {
	case <-f.entered:
	case <-time.After(time.Second):
		t.Fatal("monitor did not start")
	}
	c.Stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("controller did not stop")
	}
}
