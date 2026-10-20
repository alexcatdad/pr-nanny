package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alexcatdad/pr-nanny/internal/model"
	"github.com/alexcatdad/pr-nanny/internal/storage"
)

type memoryStore struct {
	mu      sync.Mutex
	state   storage.State
	saveErr error
}

func (s *memoryStore) LoadState() (storage.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, nil
}
func (s *memoryStore) SaveState(v storage.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.state = v
	return nil
}

type sequenceFetcher struct {
	mu      sync.Mutex
	results []model.FetchResult
	errs    []error
	calls   int
}

func (f *sequenceFetcher) Fetch(context.Context, string, string, []string) (model.FetchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.calls
	f.calls++
	if i < len(f.errs) && f.errs[i] != nil {
		return model.FetchResult{}, f.errs[i]
	}
	if i >= len(f.results) {
		return model.FetchResult{}, nil
	}
	return f.results[i], nil
}

type recorded struct {
	mu     sync.Mutex
	events []Event
}

type classifiedTestError struct {
	kind  string
	delay time.Duration
}

func (e classifiedTestError) Error() string             { return "sensitive upstream details" }
func (e classifiedTestError) Kind() string              { return e.kind }
func (e classifiedTestError) RetryDelay() time.Duration { return e.delay }

func (n *recorded) Notify(_ model.PullRequest, e Event) {
	n.mu.Lock()
	n.events = append(n.events, e)
	n.mu.Unlock()
}
func (n *recorded) list() []Event {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]Event(nil), n.events...)
}

func TestBaselineReviewReRequestAndBellGating(t *testing.T) {
	pr := model.PullRequest{ID: "PR_1", Authored: true, ReviewRequested: true, CI: model.CIPassed}
	prRemoved := pr
	prRemoved.ReviewRequested = false
	prAgain := pr
	f := &sequenceFetcher{results: []model.FetchResult{{PullRequests: []model.PullRequest{pr}}, {PullRequests: []model.PullRequest{prRemoved}}, {PullRequests: []model.PullRequest{prAgain}}}}
	s := &memoryStore{}
	n := &recorded{}
	m, err := New(f, func(context.Context) (string, string, error) { return "synthetic-token", "acct", nil }, s, n, model.DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(n.list()) != 0 {
		t.Fatalf("initial sync notified: %v", n.list())
	}
	if err = m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []Event{EventReviewRequested}
	if got := n.list(); !equalEvents(got, want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
	if err = m.SetBell(pr.ID, true); err != nil {
		t.Fatal(err)
	}
	prAgain.ReviewRequested = false
	f.results = append(f.results, model.FetchResult{PullRequests: []model.PullRequest{prAgain}})
	if err = m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	prAgain.ReviewRequested = true
	prAgain.CI = model.CINotPassed
	f.results = append(f.results, model.FetchResult{PullRequests: []model.PullRequest{prAgain}})
	if err = m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	want = append(want, EventReviewRequested, EventCI)
	if got := n.list(); !equalEvents(got, want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
}

func TestFailedFetchPreservesBaselineAndDoesNotNotify(t *testing.T) {
	baseline := model.PullRequest{ID: "p", CI: model.CIPassed, Authored: true}
	changed := baseline
	changed.CI = model.CINotPassed
	f := &sequenceFetcher{results: []model.FetchResult{{PullRequests: []model.PullRequest{baseline}}, {PullRequests: []model.PullRequest{changed}}, {PullRequests: []model.PullRequest{changed}}}, errs: []error{nil, errors.New("private raw error"), nil}}
	s := &memoryStore{}
	n := &recorded{}
	settings := model.DefaultSettings()
	m, _ := New(f, func(context.Context) (string, string, error) { return "", "", nil }, s, n, settings)
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.SetBell("p", true); err != nil {
		t.Fatal(err)
	}
	if err := m.Refresh(context.Background()); err == nil {
		t.Fatal("expected fetch error")
	}
	if len(n.list()) != 0 {
		t.Fatalf("notification on failed fetch: %v", n.list())
	}
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := n.list(); !equalEvents(got, []Event{EventCI}) {
		t.Fatalf("events=%v", got)
	}
	if got := m.Snapshot(); got.Connection != "connected" || got.Message != "" {
		t.Fatalf("not recovered: %#v", got)
	}
}

func TestGlobalNotificationsAndInaccessibleRetention(t *testing.T) {
	pr := model.PullRequest{ID: "p", CI: model.CIPassed, Authored: true}
	f := &sequenceFetcher{results: []model.FetchResult{{PullRequests: []model.PullRequest{pr}}, {InaccessibleIDs: []string{"p"}}}}
	s := &memoryStore{}
	n := &recorded{}
	settings := model.DefaultSettings()
	m, _ := New(f, func(context.Context) (string, string, error) { return "", "", nil }, s, n, settings)
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.SetBell("p", true); err != nil {
		t.Fatal(err)
	}
	settings.Notifications = false
	m.SetSettings(settings)
	changed := pr
	changed.CI = model.CINotPassed
	f.results = append(f.results, model.FetchResult{PullRequests: []model.PullRequest{changed}})
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(n.list()) != 0 {
		t.Fatalf("global off notified: %v", n.list())
	}
	if len(m.Snapshot().PullRequests) != 1 {
		t.Fatal("inaccessible PR was dropped")
	}
}

func TestRestartDoesNotDuplicateUnchangedEvent(t *testing.T) {
	base := model.PullRequest{ID: "persisted", CI: model.CIPassed, Authored: true}
	changed := base
	changed.CI = model.CINotPassed
	s := &memoryStore{}
	if err := s.SaveState(storage.State{Bells: map[string]bool{base.ID: true}, Baseline: map[string]storage.Observed{base.ID: observe(base)}, Initialized: true}); err != nil {
		t.Fatal(err)
	}
	f := &sequenceFetcher{results: []model.FetchResult{{PullRequests: []model.PullRequest{changed}}, {PullRequests: []model.PullRequest{changed}}}}
	n := &recorded{}
	newMonitor := func() *Monitor {
		m, err := New(f, func(context.Context) (string, string, error) { return "", "", nil }, s, n, model.DefaultSettings())
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	if err := newMonitor().Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := newMonitor().Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := n.list(); !equalEvents(got, []Event{EventCI}) {
		t.Fatalf("restart duplicated events: %v", got)
	}
}

func TestPersistenceFailureSuppressesNotifications(t *testing.T) {
	base := model.PullRequest{ID: "p", CI: model.CIPassed, Authored: true}
	changed := base
	changed.CI = model.CINotPassed
	f := &sequenceFetcher{results: []model.FetchResult{{PullRequests: []model.PullRequest{base}}, {PullRequests: []model.PullRequest{changed}}, {PullRequests: []model.PullRequest{changed}}}}
	s := &memoryStore{}
	n := &recorded{}
	m, _ := New(f, func(context.Context) (string, string, error) { return "", "", nil }, s, n, model.DefaultSettings())
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.SetBell("p", true); err != nil {
		t.Fatal(err)
	}
	s.saveErr = errors.New("disk unavailable")
	if err := m.Refresh(context.Background()); err == nil {
		t.Fatal("expected save failure")
	}
	if len(n.list()) != 0 {
		t.Fatalf("notified before persistence committed: %v", n.list())
	}
	s.saveErr = nil
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := n.list(); !equalEvents(got, []Event{EventCI}) {
		t.Fatalf("events=%v", got)
	}
}

func TestRecentUsesAuthoritativeClosureTime(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	closed := now.Add(-59 * time.Second)
	expired := now.Add(-61 * time.Second)
	m := &Monitor{now: func() time.Time { return now }, settings: model.DefaultSettings(), prs: map[string]model.PullRequest{
		"recent": {ID: "recent", ClosedAt: &closed}, "old": {ID: "old", ClosedAt: &expired}, "open": {ID: "open", Authored: true},
	}, state: storage.State{Bells: map[string]bool{}}}
	got := m.Snapshot().PullRequests
	if len(got) != 2 {
		t.Fatalf("recent rows=%#v", got)
	}
}

func TestCIFailPassFailAndDuplicateRows(t *testing.T) {
	base := model.PullRequest{ID: "cycle", Authored: true, CI: model.CINotPassed, UpdatedAt: time.Unix(1, 0)}
	passed := base
	passed.CI = model.CIPassed
	passed.UpdatedAt = time.Unix(2, 0)
	failAgain := base
	failAgain.UpdatedAt = time.Unix(3, 0)
	f := &sequenceFetcher{results: []model.FetchResult{{PullRequests: []model.PullRequest{base}}, {PullRequests: []model.PullRequest{passed, passed}}, {PullRequests: []model.PullRequest{failAgain, failAgain}}}}
	s := &memoryStore{}
	n := &recorded{}
	m, _ := New(f, func(context.Context) (string, string, error) { return "", "acct", nil }, s, n, model.DefaultSettings())
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.SetBell(base.ID, true); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := m.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := n.list(); !equalEvents(got, []Event{EventCI, EventCI}) {
		t.Fatalf("CI sequence/duplicates events=%v", got)
	}
}

func TestRemovedReviewRequestThenRediscoveryNotifies(t *testing.T) {
	requested := model.PullRequest{ID: "review", ReviewRequested: true, CI: model.CIPassed}
	removed := requested
	removed.ReviewRequested = false
	f := &sequenceFetcher{results: []model.FetchResult{{PullRequests: []model.PullRequest{requested}}, {PullRequests: []model.PullRequest{removed}}, {PullRequests: []model.PullRequest{requested}}}}
	s := &memoryStore{}
	n := &recorded{}
	m, _ := New(f, func(context.Context) (string, string, error) { return "", "acct", nil }, s, n, model.DefaultSettings())
	for i := 0; i < 3; i++ {
		if err := m.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := n.list(); !equalEvents(got, []Event{EventReviewRequested}) {
		t.Fatalf("review removal/re-request events=%v", got)
	}
	state := s.state
	if _, ok := state.Baseline[requested.ID]; !ok {
		t.Fatal("re-discovered review request was not baselined")
	}
}

func TestInaccessibleBaselineSurvivesRestart(t *testing.T) {
	pr := model.PullRequest{ID: "private", Authored: true, CI: model.CIPassed}
	s := &memoryStore{state: storage.State{Initialized: true, Account: "acct", Bells: map[string]bool{}, Baseline: map[string]storage.Observed{pr.ID: observe(pr)}}}
	f := &sequenceFetcher{results: []model.FetchResult{{InaccessibleIDs: []string{pr.ID}}, {PullRequests: []model.PullRequest{pr}}}}
	n := &recorded{}
	newMonitor := func() *Monitor {
		settings := model.DefaultSettings()
		settings.Account = "acct"
		m, err := New(f, func(context.Context) (string, string, error) { return "", "acct", nil }, s, n, settings)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	if err := newMonitor().Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.state.Baseline[pr.ID]; !ok {
		t.Fatal("inaccessible baseline was pruned")
	}
	if err := newMonitor().Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(n.list()) != 0 {
		t.Fatalf("recovery produced duplicate event: %v", n.list())
	}
}

func TestExpiredTerminalStateIsPrunedOnRestart(t *testing.T) {
	closedAt := time.Now().Add(-61 * time.Second)
	id := "closed-old"
	s := &memoryStore{state: storage.State{Account: "acct", Initialized: true, Bells: map[string]bool{id: true}, Baseline: map[string]storage.Observed{id: {Closed: true, ClosedAt: &closedAt}}}}
	settings := model.DefaultSettings()
	settings.Account = "acct"
	m, err := New(&sequenceFetcher{}, func(context.Context) (string, string, error) { return "", "acct", nil }, s, nil, settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.state.Baseline[id]; ok {
		t.Fatal("expired closed baseline retained")
	}
	if s.state.Bells[id] {
		t.Fatal("expired closed bell retained")
	}
	if len(m.Snapshot().PullRequests) != 0 {
		t.Fatal("closed PR displayed after expiration")
	}
}

func TestRateLimitDelayBlocksManualRefreshAndSafeMessage(t *testing.T) {
	f := &sequenceFetcher{errs: []error{classifiedTestError{kind: "rate_limit", delay: 6 * time.Hour}}}
	m, _ := New(f, func(context.Context) (string, string, error) { return "", "acct", nil }, &memoryStore{}, &recorded{}, model.DefaultSettings())
	clock := time.Now()
	m.now = func() time.Time { return clock }
	if err := m.Refresh(context.Background()); err == nil {
		t.Fatal("expected rate-limit failure")
	}
	if got := m.Snapshot(); got.Connection != "rate_limited" || got.Message != "GitHub temporarily limited requests" || got.Message == "sensitive upstream details" {
		t.Fatalf("unsafe/missing rate-limit status: %#v", got)
	}
	if err := m.Refresh(context.Background()); !errors.Is(err, ErrBackoff) {
		t.Fatalf("manual refresh error=%v", err)
	}
	if f.calls != 1 {
		t.Fatalf("backoff did not prevent request; calls=%d", f.calls)
	}
	clock = clock.Add(6 * time.Hour)
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSetBellRejectsUnknownAndRollsBackFailure(t *testing.T) {
	pr := model.PullRequest{ID: "known", Authored: true}
	f := &sequenceFetcher{results: []model.FetchResult{{PullRequests: []model.PullRequest{pr}}}}
	s := &memoryStore{}
	m, _ := New(f, func(context.Context) (string, string, error) { return "", "acct", nil }, s, &recorded{}, model.DefaultSettings())
	if err := m.SetBell("unknown", true); err == nil {
		t.Fatal("unknown PR bell accepted")
	}
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.saveErr = errors.New("disk unavailable")
	if err := m.SetBell(pr.ID, true); err == nil {
		t.Fatal("expected persistence error")
	}
	if m.Snapshot().PullRequests[0].Bell || m.state.Bells[pr.ID] || s.state.Bells[pr.ID] {
		t.Fatal("failed save left bell enabled in memory")
	}
}

func TestAccountChangeDuringFetchDiscardsStaleSnapshot(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	f := blockingFetcher{started: started, release: release, pr: model.PullRequest{ID: "old", Authored: true}}
	s := &memoryStore{state: storage.State{Account: "acct-one", Initialized: true, Bells: map[string]bool{}, Baseline: map[string]storage.Observed{}}}
	m, _ := New(f, func(context.Context) (string, string, error) { return "", "acct-one", nil }, s, &recorded{}, model.Settings{Account: "acct-one", AuthMethod: "gh", IntervalSeconds: 60, Layout: "all", Notifications: true})
	done := make(chan error, 1)
	go func() { done <- m.Refresh(context.Background()) }()
	<-started
	if err := m.Refresh(context.Background()); !errors.Is(err, ErrRefreshInProgress) {
		t.Fatalf("overlapping refresh error=%v", err)
	}
	if err := m.SetSettings(model.Settings{Account: "acct-two", AuthMethod: "pat", IntervalSeconds: 60, Layout: "all", Notifications: true}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("stale account result committed")
	}
	if len(m.Snapshot().PullRequests) != 0 || s.state.Account != "acct-two" {
		t.Fatalf("wrong account state after in-flight response: %#v %#v", m.Snapshot(), s.state)
	}
}

type blockingFetcher struct {
	started chan struct{}
	release chan struct{}
	pr      model.PullRequest
}

type signalFetcher struct{ calls chan struct{} }

func (f signalFetcher) Fetch(context.Context, string, string, []string) (model.FetchResult, error) {
	f.calls <- struct{}{}
	return model.FetchResult{}, nil
}

func TestRunWakesForIntervalChangeAndExits(t *testing.T) {
	calls := make(chan struct{}, 4)
	settings := model.DefaultSettings()
	settings.IntervalSeconds = 10
	m, _ := New(signalFetcher{calls}, func(context.Context) (string, string, error) { return "", "acct", nil }, &memoryStore{}, nil, settings)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal("initial refresh did not run")
	}
	settings.IntervalSeconds = 1
	if err := m.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	select {
	case <-calls:
	case <-time.After(1800 * time.Millisecond):
		t.Fatal("interval change did not wake polling timer")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("polling did not stop after cancellation")
	}
}

func TestSameAccountAuthSwitchKeepsBaselineButDifferentAccountClears(t *testing.T) {
	pr := model.PullRequest{ID: "p", Authored: true, CI: model.CIPassed}
	state := storage.State{Account: "acct-one", Initialized: true, Bells: map[string]bool{"p": true}, Baseline: map[string]storage.Observed{"p": observe(pr)}}
	s := &memoryStore{state: state}
	settings := model.DefaultSettings()
	settings.Account = "acct-one"
	m, err := New(&sequenceFetcher{}, func(context.Context) (string, string, error) { return "", "acct-one", nil }, s, nil, settings)
	if err != nil {
		t.Fatal(err)
	}
	settings.AuthMethod = "pat"
	if err := m.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.state.Baseline["p"]; !ok {
		t.Fatal("same-account auth switch erased baseline")
	}
	settings.Account = "acct-two"
	if err := m.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if len(m.state.Baseline) != 0 || len(m.state.Bells) != 0 || m.state.Account != "acct-two" {
		t.Fatalf("account switch did not clear state: %#v", m.state)
	}
}

func (f blockingFetcher) Fetch(context.Context, string, string, []string) (model.FetchResult, error) {
	close(f.started)
	<-f.release
	return model.FetchResult{PullRequests: []model.PullRequest{f.pr}}, nil
}

func equalEvents(a, b []Event) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReviewRemovalKeepsExplicitBellAcrossRestart(t *testing.T) {
	requested := model.PullRequest{ID: "bell-review", ReviewRequested: true, CI: model.CIPassed}
	removed := requested
	removed.ReviewRequested = false
	changed := removed
	changed.CI = model.CINotPassed
	s := &memoryStore{}
	n := &recorded{}
	f := &sequenceFetcher{results: []model.FetchResult{{PullRequests: []model.PullRequest{requested}}, {PullRequests: []model.PullRequest{removed}}}}
	creds := func(context.Context) (string, string, error) { return "", "account", nil }
	m, _ := New(f, creds, s, n, model.DefaultSettings())
	_ = m.Refresh(context.Background())
	if err := m.SetBell(requested.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := m.Snapshot().PullRequests; len(got) != 0 {
		t.Fatalf("categoryless PR displayed: %v", got)
	}
	if !s.state.Bells[requested.ID] {
		t.Fatal("explicit bell was lost")
	}
	later := &sequenceFetcher{results: []model.FetchResult{{PullRequests: []model.PullRequest{changed}}, {PullRequests: []model.PullRequest{requested}}}}
	restarted, _ := New(later, creds, s, n, model.DefaultSettings())
	_ = restarted.Refresh(context.Background())
	_ = restarted.Refresh(context.Background())
	if got := n.list(); !equalEvents(got, []Event{EventCI, EventReviewRequested, EventCI}) {
		t.Fatalf("events %v", got)
	}
	if !restarted.Snapshot().PullRequests[0].Bell {
		t.Fatal("bell not restored")
	}
}
