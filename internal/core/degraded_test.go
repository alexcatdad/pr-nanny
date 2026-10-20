package core

import (
	"context"
	"strings"
	"testing"

	"github.com/alexcatdad/pr-nanny/internal/model"
)

func degradedMonitor(t *testing.T, f *sequenceFetcher) (*Monitor, *memoryStore, *recorded) {
	t.Helper()
	s, n := &memoryStore{}, &recorded{}
	m, err := New(f, func(context.Context) (string, string, error) { return "", "account", nil }, s, n, model.DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	return m, s, n
}

// A PR seen only through a team request keeps its state while team access is
// unavailable, produces no events, and recovery replays nothing.
func TestTeamAccessLossAndRecoveryProduceNoSpuriousNotifications(t *testing.T) {
	full := model.PullRequest{ID: "team-only", ReviewRequested: true, ReviewRequestIDs: []string{"rr-team"}}
	lost := model.PullRequest{ID: "team-only", UnverifiedTeamRequestIDs: []string{"rr-team"}} // not matched while degraded
	warn := []string{model.WarningTeamReview}
	f := &sequenceFetcher{results: []model.FetchResult{
		{PullRequests: []model.PullRequest{full}},
		{PullRequests: []model.PullRequest{lost}, Warnings: warn},
		{PullRequests: []model.PullRequest{lost}, Warnings: warn},
		{PullRequests: []model.PullRequest{full}},
	}}
	m, s, n := degradedMonitor(t, f)
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := n.list(); len(got) != 0 {
		t.Fatalf("initial baseline notified: %v", got)
	}
	for i := 0; i < 2; i++ {
		if err := m.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
		snap := m.Snapshot()
		if !snap.Degraded || !strings.Contains(snap.Message, "Team review") || snap.Connection != "connected" {
			t.Fatalf("partial sync must be flagged: %+v", snap)
		}
		if len(snap.PullRequests) != 1 || !snap.PullRequests[0].ReviewRequested {
			t.Fatalf("team-only PR dropped or demoted while degraded: %+v", snap.PullRequests)
		}
		if b := s.state.Baseline["team-only"]; !b.ReviewRequested || len(b.ReviewRequestIDs) != 1 {
			t.Fatalf("baseline lost the team request: %+v", b)
		}
	}
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if snap := m.Snapshot(); snap.Degraded || snap.Message != "" {
		t.Fatalf("recovered sync still flagged: %+v", snap)
	}
	if got := n.list(); len(got) != 0 {
		t.Fatalf("loss or recovery produced notifications: %v", got)
	}
}

// A team request that really disappeared must still be recognized as removed
// even in degraded mode, because its node ID is no longer present.
func TestDegradedModeStillDropsRequestsThatNoLongerExist(t *testing.T) {
	full := model.PullRequest{ID: "pr", Authored: true, ReviewRequested: true, ReviewRequestIDs: []string{"rr-team"}}
	gone := model.PullRequest{ID: "pr", Authored: true}
	f := &sequenceFetcher{results: []model.FetchResult{
		{PullRequests: []model.PullRequest{full}},
		{PullRequests: []model.PullRequest{gone}, Warnings: []string{model.WarningTeamReview}},
	}}
	m, s, _ := degradedMonitor(t, f)
	for range f.results {
		if err := m.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if s.state.Baseline["pr"].ReviewRequested {
		t.Fatal("a request absent from GitHub was carried forward")
	}
}

// A request first seen while degraded is not invented, and one that arrives
// after recovery is a genuine first notification.
func TestDegradedModeDoesNotInventRequestsAndRecoveryNotifiesOnlyNew(t *testing.T) {
	unseen := model.PullRequest{ID: "new-team", UnverifiedTeamRequestIDs: []string{"rr-new"}}
	recovered := model.PullRequest{ID: "new-team", ReviewRequested: true, ReviewRequestIDs: []string{"rr-new"}}
	f := &sequenceFetcher{results: []model.FetchResult{
		{},
		{PullRequests: []model.PullRequest{unseen}, Warnings: []string{model.WarningTeamReview}},
		{PullRequests: []model.PullRequest{recovered}},
	}}
	m, _, n := degradedMonitor(t, f)
	for range f.results {
		if err := m.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := n.list(); !equalEvents(got, []Event{EventReviewRequested}) {
		t.Fatalf("events = %v", got)
	}
}

func TestDegradedFlagClearsOnFailure(t *testing.T) {
	f := &sequenceFetcher{results: []model.FetchResult{{Warnings: []string{model.WarningTeamReview}}}, errs: []error{nil, classifiedTestError{kind: "offline"}}}
	m, _, _ := degradedMonitor(t, f)
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !m.Snapshot().Degraded {
		t.Fatal("expected degraded")
	}
	_ = m.Refresh(context.Background())
	if snap := m.Snapshot(); snap.Degraded || snap.Connection != "offline" {
		t.Fatalf("failure must replace degraded state: %+v", snap)
	}
}
