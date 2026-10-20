package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alexcatdad/pr-nanny/internal/model"
)

// This advances ten hours of polling time without sleeping or contacting GitHub.
// It exercises repeated outage/recovery and restart deduplication, not wall-time
// OS soak acceptance or native resource accounting.
func TestSustainedPollingRecoversWithoutReplayOrStateGrowth(t *testing.T) {
	const cycles = 600
	clock := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	fetcher := &sequenceFetcher{results: make([]model.FetchResult, cycles+1), errs: make([]error, cycles+1)}
	for i := range cycles {
		ci := model.CIPassed
		if (i/20)%2 == 1 {
			ci = model.CINotPassed
		}
		fetcher.results[i] = model.FetchResult{PullRequests: []model.PullRequest{{
			ID: "steady-pr", Authored: true, CI: ci, UpdatedAt: clock.Add(time.Duration(i) * time.Minute),
		}}}
		if i > 0 && i%17 == 0 {
			fetcher.errs[i] = errors.New("synthetic network outage")
		}
	}
	store := &memoryStore{}
	notifications := &recorded{}
	settings := model.DefaultSettings()
	settings.Account = "fixture-account"
	credentials := func(context.Context) (string, string, error) { return "fixture-token", settings.Account, nil }
	monitor, err := New(fetcher, credentials, store, notifications, settings)
	if err != nil {
		t.Fatal(err)
	}
	monitor.now = func() time.Time { return clock }
	lastCI := model.CI("")
	expectedNotifications := 0
	for i := range cycles {
		err := monitor.Refresh(context.Background())
		if fetcher.errs[i] != nil {
			if err == nil || monitor.Snapshot().Connection != "offline" {
				t.Fatalf("cycle %d: outage was not reported", i)
			}
		} else {
			if err != nil {
				t.Fatalf("cycle %d: recovery failed: %v", i, err)
			}
			ci := fetcher.results[i].PullRequests[0].CI
			if i > 0 && ci != lastCI {
				expectedNotifications++
			}
			lastCI = ci
		}
		snapshot := monitor.Snapshot()
		if len(snapshot.PullRequests) != 1 || snapshot.PullRequests[0].CI != lastCI {
			t.Fatalf("cycle %d: last successful snapshot was lost", i)
		}
		if i == 0 {
			if err := monitor.SetBell("steady-pr", true); err != nil {
				t.Fatal(err)
			}
		}
		clock = clock.Add(time.Minute)
	}
	if len(notifications.list()) != expectedNotifications {
		t.Fatalf("notification attempts = %d, want %d", len(notifications.list()), expectedNotifications)
	}
	if len(store.state.Baseline) != 1 || len(store.state.Bells) != 1 {
		t.Fatal("steady polling grew the persisted PR identity state")
	}
	fetcher.results[cycles] = fetcher.results[cycles-1]
	restarted, err := New(fetcher, credentials, store, notifications, settings)
	if err != nil {
		t.Fatal(err)
	}
	restarted.now = func() time.Time { return clock }
	if err := restarted.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifications.list()) != expectedNotifications || !restarted.Snapshot().PullRequests[0].Bell {
		t.Fatal("restart replayed a notification or lost the enabled bell")
	}
}
