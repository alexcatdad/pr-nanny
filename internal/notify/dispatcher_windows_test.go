//go:build windows

package notify

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/alexcatdad/pr-nanny/internal/core"
	"github.com/alexcatdad/pr-nanny/internal/model"
	"golang.org/x/sys/windows"
)

func TestWindowsDeliveriesUseOneOSThread(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	threadIDs := make(chan uint32, 1)
	d := New(ctx, func(model.PullRequest, core.Event) {
		runtime.Gosched()
		threadIDs <- windows.GetCurrentThreadId()
	}, nil)
	var first uint32
	for i := 0; i < 32; i++ {
		d.Notify(model.PullRequest{Number: i + 1}, core.EventCI)
		select {
		case id := <-threadIDs:
			if i == 0 {
				first = id
			} else if id != first {
				t.Fatalf("notification moved from OS thread %d to %d", first, id)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("notification delivery stalled")
		}
	}
	cancel()
	select {
	case <-d.done:
	case <-time.After(5 * time.Second):
		t.Fatal("notification thread did not stop")
	}
}
