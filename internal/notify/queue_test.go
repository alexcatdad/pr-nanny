package notify

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexcatdad/pr-nanny/internal/core"
	"github.com/alexcatdad/pr-nanny/internal/model"
)

func queuePR(id string) model.PullRequest {
	return model.PullRequest{ID: id, Repository: "acme/widgets", Number: 42, Title: "Add a widget", UpdatedAt: time.Unix(1700000000, 0).UTC(), CI: model.CIPassed,
		ReviewRequestIDs: []string{"RR_synthetic"}, UnverifiedTeamRequestIDs: []string{"TEAM_synthetic"}}
}

func allowAll(string, core.Event) bool { return true }

func TestQueueDeliversNothingWithoutFilter(t *testing.T) {
	q := NewQueue()
	q.Notify(queuePR("pr-1"), core.EventCI)
	if got := q.Next("", 0); len(got.Notifications) != 0 {
		t.Fatalf("delivered without a gate: %+v", got)
	}
}

func TestQueueUsesFixedTitlesAndStableDedupeIDs(t *testing.T) {
	q := NewQueue()
	q.SetFilter(allowAll)
	pr := queuePR("pr-1")
	q.Notify(pr, core.EventReviewRequested)
	q.Notify(pr, core.Event("unknown"))
	got := q.Next("", 0).Notifications
	if len(got) != 1 {
		t.Fatalf("notifications = %+v", got)
	}
	n := got[0]
	if n.Title != "Review requested" || n.Subtitle != "acme/widgets #42" || n.Body != "Add a widget" || n.PRID != "pr-1" || n.Event != "review_requested" {
		t.Fatalf("notification = %+v", n)
	}
	if n.ID != DedupeID(pr, core.EventReviewRequested) || len(n.ID) != 32 {
		t.Fatalf("dedupe ID = %q", n.ID)
	}
	data, _ := json.Marshal(n)
	if strings.Contains(string(data), "synthetic") {
		t.Fatalf("internal request IDs crossed the boundary: %s", data)
	}
}

func TestQueueSkipsRetainedDuplicates(t *testing.T) {
	q := NewQueue()
	q.SetFilter(allowAll)
	pr := queuePR("pr-1")
	q.Notify(pr, core.EventCI)
	q.Notify(pr, core.EventCI)
	pr.CI = model.CINotPassed
	q.Notify(pr, core.EventCI)
	if got := q.Next("", 0).Notifications; len(got) != 2 || got[0].ID == got[1].ID {
		t.Fatalf("notifications = %+v", got)
	}
}

func TestQueueCursorAdvancesAndConsumesGatedEntries(t *testing.T) {
	q := NewQueue()
	var allowed atomic.Bool
	q.SetFilter(func(string, core.Event) bool { return allowed.Load() })
	q.Notify(queuePR("pr-1"), core.EventCI)
	first := q.Next("", 0)
	if len(first.Notifications) != 0 {
		t.Fatalf("gated notification delivered: %+v", first)
	}
	allowed.Store(true)
	// A rejected entry is consumed: re-enabling settings does not replay it.
	if got := q.Next(first.Cursor, 0); len(got.Notifications) != 0 || got.Cursor != first.Cursor {
		t.Fatalf("rejected entry replayed: %+v (cursor %q)", got, first.Cursor)
	}
	q.Notify(queuePR("pr-2"), core.EventMerged)
	second := q.Next(first.Cursor, 0)
	if len(second.Notifications) != 1 || second.Notifications[0].PRID != "pr-2" || second.Missed {
		t.Fatalf("second read = %+v", second)
	}
	if got := q.Next(second.Cursor, 0); len(got.Notifications) != 0 {
		t.Fatalf("read past cursor repeated: %+v", got)
	}
}

func TestQueueSettingChangedAfterQueueingSuppressesDelivery(t *testing.T) {
	q := NewQueue()
	var enabled atomic.Bool
	enabled.Store(true)
	q.SetFilter(func(string, core.Event) bool { return enabled.Load() })
	q.Notify(queuePR("pr-1"), core.EventCI)
	enabled.Store(false)
	if got := q.Next("", 0); len(got.Notifications) != 0 {
		t.Fatalf("notification delivered after disabling: %+v", got)
	}
}

func TestQueueGateSeesEachEntry(t *testing.T) {
	q := NewQueue()
	var calls []string
	q.SetFilter(func(id string, event core.Event) bool {
		calls = append(calls, id+":"+string(event))
		return id == "pr-2"
	})
	q.Notify(queuePR("pr-1"), core.EventApproval)
	q.Notify(queuePR("pr-2"), core.EventClosed)
	got := q.Next("", 0).Notifications
	if fmt.Sprint(calls) != "[pr-1:approval pr-2:closed]" || len(got) != 1 || got[0].PRID != "pr-2" {
		t.Fatalf("calls=%v notifications=%+v", calls, got)
	}
}

func TestQueueIsBoundedAndReportsMissedEntries(t *testing.T) {
	q := NewQueue()
	q.SetFilter(allowAll)
	q.Notify(queuePR("pr-0"), core.EventCI)
	start := q.Next("", 0)
	for i := 1; i <= MaxBatch+5; i++ {
		q.Notify(queuePR(fmt.Sprintf("pr-%d", i)), core.EventCI)
	}
	got := q.Next(start.Cursor, 0)
	if !got.Missed || len(got.Notifications) != MaxBatch || got.Notifications[0].PRID != "pr-6" {
		t.Fatalf("missed=%t count=%d first=%+v", got.Missed, len(got.Notifications), got.Notifications[0])
	}
	limited := q.Next(start.Cursor, 2)
	if len(limited.Notifications) != 2 {
		t.Fatalf("limit ignored: %d", len(limited.Notifications))
	}
	rest := q.Next(limited.Cursor, 0)
	if rest.Missed || len(rest.Notifications) != MaxBatch-2 {
		t.Fatalf("rest missed=%t count=%d", rest.Missed, len(rest.Notifications))
	}
}

func TestQueueStaleOrMalformedCursorReadsFromOldest(t *testing.T) {
	q := NewQueue()
	q.SetFilter(allowAll)
	q.Notify(queuePR("pr-1"), core.EventCI)
	for _, cursor := range []string{"other-1", "garbage", q.epoch + "-99", q.epoch + "-x"} {
		if got := q.Next(cursor, 0); len(got.Notifications) != 1 || got.Missed || !strings.HasPrefix(got.Cursor, q.epoch+"-") {
			t.Fatalf("cursor %q: %+v", cursor, got)
		}
	}
}
