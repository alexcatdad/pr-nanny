// Package core coordinates polling, local notification preferences, and PR state.
package core

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/alexcatdad/pr-nanny/internal/model"
	"github.com/alexcatdad/pr-nanny/internal/storage"
)

type Fetcher interface {
	Fetch(ctx context.Context, token, account string, trackedIDs []string) (model.FetchResult, error)
}

type Credentials func(context.Context) (token, account string, err error)
type Notifier interface {
	Notify(model.PullRequest, Event)
}

type Event string

const (
	EventCI              Event = "ci"
	EventApproval        Event = "approval"
	EventReviewRequested Event = "review_requested"
	EventMerged          Event = "merged"
	EventClosed          Event = "closed"
)

type Persistence interface {
	LoadState() (storage.State, error)
	SaveState(storage.State) error
}

// Classified errors let GitHub/auth adapters provide safe user-facing states
// and server-directed retry delays without leaking upstream error text.
type ClassifiedError interface {
	Kind() string
	RetryDelay() time.Duration
}

var (
	ErrBackoff           = errors.New("refresh is delayed by the server")
	ErrRefreshInProgress = errors.New("refresh already in progress")
)

type Monitor struct {
	fetcher Fetcher
	creds   Credentials
	store   Persistence
	notify  Notifier
	now     func() time.Time

	mu          sync.Mutex
	refreshMu   sync.Mutex
	wake        chan bool
	settings    model.Settings
	prs         map[string]model.PullRequest
	state       storage.State
	lastRefresh *time.Time
	connection  string
	message     string
	refreshing  bool
	retryAt     time.Time
	generation  uint64
	// pollAttemptHook is an optional synchronization seam for scheduler tests.
	pollAttemptHook func(error)
}

func New(fetcher Fetcher, creds Credentials, store Persistence, notifier Notifier, settings model.Settings) (*Monitor, error) {
	if fetcher == nil || creds == nil || store == nil {
		return nil, errors.New("monitor dependencies are required")
	}
	state, err := store.LoadState()
	if err != nil {
		return nil, err
	}
	if state.Bells == nil {
		state.Bells = map[string]bool{}
	}
	if state.Baseline == nil {
		state.Baseline = map[string]storage.Observed{}
	}
	state.Baseline = cloneBaseline(state.Baseline)
	if pruneExpiredState(&state, time.Now()) {
		if err := store.SaveState(cloneState(state)); err != nil {
			return nil, err
		}
	}
	if state.Account != "" && state.Account != settings.Account {
		state.Bells, state.Baseline, state.Initialized = map[string]bool{}, map[string]storage.Observed{}, false
		state.Account = settings.Account
		if err := store.SaveState(cloneState(state)); err != nil {
			return nil, err
		}
	}
	return &Monitor{fetcher: fetcher, creds: creds, store: store, notify: notifier, now: time.Now,
		settings: settings, prs: map[string]model.PullRequest{}, state: state, connection: "disconnected", wake: make(chan bool, 1)}, nil
}

func (m *Monitor) SetSettings(settings model.Settings) error {
	m.mu.Lock()
	old := m.settings
	if old.Account != settings.Account && (old.Account != "" || settings.Account != "") {
		previous := m.state
		m.state.Bells, m.state.Baseline, m.state.Initialized = map[string]bool{}, map[string]storage.Observed{}, false
		m.state.Account = settings.Account
		if err := m.store.SaveState(cloneState(m.state)); err != nil {
			m.state = previous
			m.mu.Unlock()
			return err
		}
		m.prs = map[string]model.PullRequest{}
		m.lastRefresh = nil
	}
	m.settings = settings
	m.generation++
	m.mu.Unlock()
	// Account/authentication changes deserve an immediate refresh; all changes
	// wake the scheduler so a new interval is applied without polling stale timers.
	if old.Account != settings.Account || old.AuthMethod != settings.AuthMethod {
		m.wakeRun(true)
	} else {
		m.wakeRun(false)
	}
	return nil
}
func (m *Monitor) SetBell(id string, enabled bool) error {
	if id == "" {
		return errors.New("pull request id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, known := m.prs[id]; !known {
		return errors.New("pull request is unavailable")
	}
	candidate := m.state
	candidate.Bells = cloneBells(m.state.Bells)
	if enabled {
		candidate.Bells[id] = true
	} else {
		delete(candidate.Bells, id)
	}
	candidate.Baseline = cloneBaseline(candidate.Baseline)
	if err := m.store.SaveState(cloneState(candidate)); err != nil {
		return err
	}
	m.state = candidate
	if pr, ok := m.prs[id]; ok {
		pr.Bell = enabled
		m.prs[id] = pr
	}
	return nil
}

// Refresh performs one serialized fetch. Failed fetches preserve the last good
// state and never advance the notification baseline.
func (m *Monitor) Refresh(ctx context.Context) error {
	if !m.refreshMu.TryLock() {
		return ErrRefreshInProgress
	}
	var generation uint64
	capturedGeneration := false
	defer func() {
		stale := false
		if capturedGeneration {
			m.mu.Lock()
			m.refreshing = false
			stale = generation != m.generation
			m.mu.Unlock()
		}
		m.refreshMu.Unlock()
		if stale {
			m.wakeRun(true)
		}
	}()
	m.mu.Lock()
	if !m.retryAt.IsZero() && m.now().Before(m.retryAt) {
		m.mu.Unlock()
		return ErrBackoff
	}
	m.refreshing = true
	generation = m.generation
	capturedGeneration = true
	m.mu.Unlock()
	token, account, err := m.creds(ctx)
	m.mu.Lock()
	if generation != m.generation {
		m.mu.Unlock()
		return errors.New("settings changed during refresh")
	}
	if err != nil {
		m.setFailureLocked(err)
		m.mu.Unlock()
		return err
	}
	if m.settings.Account != "" && account != "" && m.settings.Account != account {
		m.connection, m.message = "authentication_error", "GitHub account changed; refresh settings"
		m.mu.Unlock()
		return errors.New("authenticated account changed")
	}
	accountChanged := m.state.Account != "" && account != "" && m.state.Account != account
	if accountChanged {
		reset := storage.State{Account: account, Bells: map[string]bool{}, Baseline: map[string]storage.Observed{}, Initialized: false}
		if err := m.store.SaveState(cloneState(reset)); err != nil {
			m.setFailureLocked(err)
			m.mu.Unlock()
			return err
		}
		m.state = reset
		m.prs = map[string]model.PullRequest{}
	}
	if pruneExpiredState(&m.state, m.now()) {
		if err := m.store.SaveState(cloneState(m.state)); err != nil {
			m.setFailureLocked(err)
			m.mu.Unlock()
			return err
		}
	}
	tracked := trackedIDs(m.state.Baseline)
	m.mu.Unlock()
	result, err := m.fetcher.Fetch(ctx, token, account, tracked)
	if err != nil {
		m.mu.Lock()
		if generation != m.generation {
			m.mu.Unlock()
			return errors.New("settings changed during refresh")
		}
		m.setFailureLocked(err)
		m.mu.Unlock()
		return err
	}
	now := m.now()
	m.mu.Lock()
	if generation != m.generation || (m.settings.Account != "" && account != "" && m.settings.Account != account) {
		m.mu.Unlock()
		return errors.New("settings changed during refresh")
	}
	old := m.state
	if old.Account != "" && account != "" && old.Account != account {
		old.Baseline, old.Bells, old.Initialized = map[string]storage.Observed{}, map[string]bool{}, false
	}
	newState := storage.State{Account: account, Bells: cloneBells(old.Bells), Baseline: cloneBaseline(old.Baseline), Initialized: old.Initialized}
	updated := make(map[string]model.PullRequest, len(result.PullRequests)+len(result.InaccessibleIDs))
	var pending []pendingNotification
	for _, pr := range deduplicate(result.PullRequests) {
		if pr.ID == "" {
			continue
		}
		pr.ReviewRequestIDs = normalizeReviewRequestIDs(pr.ReviewRequestIDs)
		pr.Bell = newState.Bells[pr.ID]
		cur := observe(pr)
		prev, exists := old.Baseline[pr.ID]
		relevant := pr.Authored || pr.ReviewRequested || old.Bells[pr.ID] || (terminal(pr) && exists)
		if !relevant {
			delete(newState.Baseline, pr.ID)
			delete(newState.Bells, pr.ID)
			continue
		}
		if exists || old.Initialized {
			pending = append(pending, m.transitions(pr, prev, cur, exists)...)
		}
		newState.Baseline[pr.ID] = cur
		if !terminal(pr) || recent(pr, now) {
			updated[pr.ID] = pr
		} else {
			delete(newState.Baseline, pr.ID)
			delete(newState.Bells, pr.ID)
		}
	}
	for _, id := range result.InaccessibleIDs {
		if pr, ok := m.prs[id]; ok {
			pr.ReviewRequestIDs = append([]string(nil), pr.ReviewRequestIDs...)
			pr.CI = model.CINotAvailable
			updated[id] = pr
		}
	}
	retained := make(map[string]bool, len(updated)+len(result.InaccessibleIDs))
	for id := range updated {
		retained[id] = true
	}
	for _, id := range result.InaccessibleIDs {
		retained[id] = true
	}
	for id := range newState.Baseline {
		if !retained[id] {
			delete(newState.Baseline, id)
			delete(newState.Bells, id)
		}
	}
	newState.Initialized = true
	if err := m.store.SaveState(cloneState(newState)); err != nil {
		m.setFailureLocked(err)
		m.mu.Unlock()
		return err
	}
	m.state = newState
	m.prs = updated
	m.connection = "connected"
	if len(result.InaccessibleIDs) > 0 {
		m.message = "Some pull requests are inaccessible; cached details may be out of date."
	} else {
		m.message = ""
	}
	t := now
	m.lastRefresh = &t
	m.retryAt = time.Time{}
	m.mu.Unlock()
	for _, item := range pending {
		if m.notify != nil {
			m.notify.Notify(item.pr, item.event)
		}
	}
	return nil
}

type pendingNotification struct {
	pr    model.PullRequest
	event Event
}

func (m *Monitor) transitions(pr model.PullRequest, prev, cur storage.Observed, known bool) []pendingNotification {
	settings := m.settings
	bell := m.state.Bells[pr.ID]
	var pending []pendingNotification
	add := func(event Event) { pending = append(pending, pendingNotification{pr: pr, event: event}) }
	if !settings.Notifications {
		return nil
	}
	// First appearance establishes baseline. Review requests notify on each false->true
	// edge even when the per-PR bell is off.
	if settings.Events.ReviewRequested && reviewRequestChanged(prev, cur) {
		add(EventReviewRequested)
	}
	if !bell {
		return pending
	}
	if !known {
		return pending
	}
	if settings.Events.CI && prev.CI != cur.CI && cur.CI != "" {
		add(EventCI)
	}
	if settings.Events.Approval && prev.Approved != cur.Approved {
		add(EventApproval)
	}
	if settings.Events.Merged && !prev.Merged && cur.Merged {
		add(EventMerged)
	}
	if settings.Events.Closed && !prev.Closed && cur.Closed && !cur.Merged {
		add(EventClosed)
	}
	return pending
}

func (m *Monitor) setFailureLocked(err error) {
	m.retryAt = time.Time{}
	var classified ClassifiedError
	if errors.As(err, &classified) {
		switch classified.Kind() {
		case "auth":
			m.connection, m.message = "authentication_error", "GitHub authentication needs attention"
		case "rate_limit":
			m.connection, m.message = "rate_limited", "GitHub temporarily limited requests"
		case "offline":
			m.connection, m.message = "offline", "Unable to connect to GitHub"
		default:
			m.connection, m.message = "error", "Unable to refresh pull requests"
		}
		if delay := classified.RetryDelay(); delay > 0 {
			m.retryAt = m.now().Add(delay)
		}
		return
	}
	m.connection, m.message = "offline", "Unable to refresh pull requests"
}
func (m *Monitor) Snapshot() model.Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	prs := make([]model.PullRequest, 0, len(m.prs))
	now := m.now()
	for _, pr := range m.prs {
		if !terminal(pr) && !pr.Authored && !pr.ReviewRequested {
			continue
		}
		if terminal(pr) && !recent(pr, now) {
			continue
		}
		pr.Bell = m.state.Bells[pr.ID]
		pr.ReviewRequestIDs = append([]string(nil), pr.ReviewRequestIDs...)
		if pr.ClosedAt != nil {
			t := *pr.ClosedAt
			pr.ClosedAt = &t
		}
		prs = append(prs, pr)
	}
	sort.Slice(prs, func(i, j int) bool {
		if prs[i].UpdatedAt.Equal(prs[j].UpdatedAt) {
			return prs[i].ID < prs[j].ID
		}
		return prs[i].UpdatedAt.After(prs[j].UpdatedAt)
	})
	var last *time.Time
	if m.lastRefresh != nil {
		v := *m.lastRefresh
		last = &v
	}
	return model.Snapshot{Settings: m.settings, PullRequests: prs, Connection: m.connection, Message: m.message, LastRefresh: last, Refreshing: m.refreshing}
}

// Run polls immediately, then at the configured interval; consecutive failures
// back off exponentially up to five minutes and recover at the next successful fetch.
func (m *Monitor) Run(ctx context.Context) {
	backoff := time.Duration(0)
	shouldRefresh := true
	for {
		if shouldRefresh {
			err := m.Refresh(ctx)
			if m.pollAttemptHook != nil {
				m.pollAttemptHook(err)
			}
			if err != nil && !errors.Is(err, ErrBackoff) && !errors.Is(err, ErrRefreshInProgress) {
				if backoff == 0 {
					backoff = 5 * time.Second
				} else {
					backoff *= 2
				}
				if backoff > 5*time.Minute {
					backoff = 5 * time.Minute
				}
			} else if err == nil {
				backoff = 0
			}
		}
		shouldRefresh = true
		m.mu.Lock()
		interval := time.Duration(m.settings.IntervalSeconds) * time.Second
		m.mu.Unlock()
		if interval <= 0 {
			interval = time.Minute
		}
		if backoff > interval {
			interval = backoff
		}
		m.mu.Lock()
		retryWait := m.retryAt.Sub(m.now())
		m.mu.Unlock()
		if retryWait > interval {
			interval = retryWait
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		case immediate := <-m.wake:
			timer.Stop()
			shouldRefresh = immediate
			if immediate {
				backoff = 0
			}
		}
	}
}

func (m *Monitor) wakeRun(immediate bool) {
	if immediate {
		select {
		case m.wake <- true:
			return
		default:
		}
		select {
		case <-m.wake:
		default:
		}
		select {
		case m.wake <- true:
		default:
		}
		return
	}
	// A queued immediate refresh is stronger than an interval-only wake.
	select {
	case m.wake <- false:
	default:
	}
}

func trackedIDs(baseline map[string]storage.Observed) []string {
	ids := make([]string, 0, len(baseline))
	for id := range baseline {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
func terminal(pr model.PullRequest) bool { return pr.ClosedAt != nil || pr.Merged }
func recent(pr model.PullRequest, now time.Time) bool {
	return pr.ClosedAt != nil && !now.Before(*pr.ClosedAt) && now.Sub(*pr.ClosedAt) < 60*time.Second
}
func deduplicate(prs []model.PullRequest) []model.PullRequest {
	byID := make(map[string]model.PullRequest, len(prs))
	for _, pr := range prs {
		if pr.ID == "" {
			continue
		}
		pr.ReviewRequestIDs = append([]string(nil), pr.ReviewRequestIDs...)
		if prior, ok := byID[pr.ID]; !ok || pr.UpdatedAt.After(prior.UpdatedAt) {
			byID[pr.ID] = pr
		}
	}
	out := make([]model.PullRequest, 0, len(byID))
	for _, pr := range byID {
		out = append(out, pr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func observe(pr model.PullRequest) storage.Observed {
	var closedAt *time.Time
	if pr.ClosedAt != nil {
		v := *pr.ClosedAt
		closedAt = &v
	}
	return storage.Observed{CI: pr.CI, Approved: pr.Approved, ReviewRequested: pr.ReviewRequested,
		ReviewRequestIDs: normalizeReviewRequestIDs(pr.ReviewRequestIDs), Merged: pr.Merged, Closed: pr.ClosedAt != nil, ClosedAt: closedAt}
}

func reviewRequestChanged(prev, cur storage.Observed) bool {
	if !prev.ReviewRequested && cur.ReviewRequested {
		return true
	}
	// Older baseline versions knew only the boolean. Seed the new identity set
	// without replaying a request that was already active during the upgrade.
	if !prev.ReviewRequested || !cur.ReviewRequested || len(prev.ReviewRequestIDs) == 0 {
		return false
	}
	previous := make(map[string]struct{}, len(prev.ReviewRequestIDs))
	for _, id := range prev.ReviewRequestIDs {
		previous[id] = struct{}{}
	}
	for _, id := range cur.ReviewRequestIDs {
		if _, known := previous[id]; !known {
			return true
		}
	}
	return false
}

func normalizeReviewRequestIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			set[id] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func pruneExpiredState(state *storage.State, now time.Time) bool {
	changed := false
	for id, observed := range state.Baseline {
		if observed.Closed || observed.Merged {
			if observed.ClosedAt == nil || now.Before(*observed.ClosedAt) || now.Sub(*observed.ClosedAt) >= 60*time.Second {
				delete(state.Baseline, id)
				delete(state.Bells, id)
				changed = true
			}
		}
	}
	return changed
}
func cloneBells(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func cloneBaseline(in map[string]storage.Observed) map[string]storage.Observed {
	out := make(map[string]storage.Observed, len(in))
	for k, v := range in {
		v.ReviewRequestIDs = append([]string(nil), v.ReviewRequestIDs...)
		if v.ClosedAt != nil {
			t := *v.ClosedAt
			v.ClosedAt = &t
		}
		out[k] = v
	}
	return out
}

func cloneState(in storage.State) storage.State {
	in.Bells = cloneBells(in.Bells)
	in.Baseline = cloneBaseline(in.Baseline)
	return in
}
