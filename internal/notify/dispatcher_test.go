package notify

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alexcatdad/pr-nanny/internal/core"
	"github.com/alexcatdad/pr-nanny/internal/model"
)

func TestBlockedDeliveryDoesNotBlockNotifyAndQueueIsBounded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	var delivered []int
	overflow := make(chan struct{}, 1)
	drained := make(chan struct{})
	d := New(ctx, func(pr model.PullRequest, _ core.Event) {
		if pr.Number == 1 {
			close(started)
			<-release
		}
		mu.Lock()
		delivered = append(delivered, pr.Number)
		mu.Unlock()
		if pr.Number == queueSize+1 {
			close(drained)
		}
	}, func() { overflow <- struct{}{} })

	d.Notify(model.PullRequest{Number: 1}, core.EventCI)
	<-started
	for n := 2; n < queueSize+2; n++ {
		d.Notify(model.PullRequest{Number: n}, core.EventCI)
	}
	// One more than the worker-held item plus queue capacity must be dropped.
	d.Notify(model.PullRequest{Number: queueSize + 2}, core.EventCI)
	if got := len(d.queue); got != queueSize {
		t.Fatalf("queue length %d, want bounded capacity %d", got, queueSize)
	}
	select {
	case <-overflow:
		t.Fatal("overflow callback ran concurrently with blocked delivery")
	default:
	}
	close(release)
	select {
	case <-overflow:
	case <-time.After(2 * time.Second):
		t.Fatal("overflow was not reported")
	}
	select {
	case <-d.done:
		t.Fatal("dispatcher stopped without cancellation")
	default:
	}
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("queued notifications were not drained")
	}
	cancel()
	select {
	case <-d.done:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatcher did not stop after cancellation")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(delivered) != queueSize+1 {
		t.Fatalf("delivered %d events, want first plus %d queued events", len(delivered), queueSize)
	}
	for i, n := range delivered {
		if want := i + 1; n != want {
			t.Fatalf("delivery %d has PR number %d, want %d", i, n, want)
		}
	}
}

func TestNotifyCopiesMutablePRFields(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	got := make(chan model.PullRequest, 1)
	d := New(ctx, func(pr model.PullRequest, _ core.Event) {
		if pr.Number == 1 {
			close(started)
			<-release
			return
		}
		got <- pr
	}, nil)
	d.Notify(model.PullRequest{Number: 1}, core.EventCI)
	<-started
	closed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	wantClosed := closed
	pr := model.PullRequest{Number: 2, ReviewRequestIDs: []string{"original"}, ClosedAt: &closed}
	d.Notify(pr, core.EventReviewRequested)
	pr.ReviewRequestIDs[0] = "mutated"
	*pr.ClosedAt = closed.Add(time.Hour)
	close(release)
	select {
	case gotPR := <-got:
		if gotPR.ReviewRequestIDs[0] != "original" || !gotPR.ClosedAt.Equal(wantClosed) {
			t.Fatalf("notification retained caller-owned fields: %#v", gotPR)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("notification was not delivered")
	}
	cancel()
	<-d.done
}

func TestCancellationDiscardsPendingNotifications(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	var delivered []int
	d := New(ctx, func(pr model.PullRequest, _ core.Event) {
		if pr.Number == 1 {
			close(started)
			<-release
		}
		mu.Lock()
		delivered = append(delivered, pr.Number)
		mu.Unlock()
	}, nil)
	d.Notify(model.PullRequest{Number: 1}, core.EventCI)
	<-started
	d.Notify(model.PullRequest{Number: 2}, core.EventCI)
	cancel()
	close(release)
	select {
	case <-d.done:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatcher did not stop")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(delivered) != 1 || delivered[0] != 1 {
		t.Fatalf("pending notification delivered after cancellation: %v", delivered)
	}
}
