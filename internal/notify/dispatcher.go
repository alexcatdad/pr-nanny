// Package notify provides a bounded asynchronous notification dispatcher.
package notify

import (
	"context"

	"github.com/alexcatdad/pr-nanny/internal/core"
	"github.com/alexcatdad/pr-nanny/internal/model"
)

const queueSize = 64

type item struct {
	pr    model.PullRequest
	event core.Event
}

// Dispatcher implements core.Notifier without making the polling path wait for
// a native notification API. Delivery is serial and the queue is bounded.
type Dispatcher struct {
	queue    chan item
	overflow chan struct{}
	done     chan struct{}
	deliver  func(model.PullRequest, core.Event)
	onFull   func()
}

// New starts one worker that exits when ctx is canceled. A currently-running
// deliver call cannot be interrupted, but cancellation discards queued items
// and does not wait for that native call to return.
func New(ctx context.Context, deliver func(model.PullRequest, core.Event), overflow func()) *Dispatcher {
	d := &Dispatcher{
		queue:    make(chan item, queueSize),
		overflow: make(chan struct{}, 1),
		done:     make(chan struct{}),
		deliver:  deliver,
		onFull:   overflow,
	}
	go d.run(ctx)
	return d
}

// Notify copies the event data and enqueues it, dropping it when the bounded
// queue is full. Overflow reporting is coalesced and performed by the worker.
func (d *Dispatcher) Notify(pr model.PullRequest, event core.Event) {
	pr.ReviewRequestIDs = append([]string(nil), pr.ReviewRequestIDs...)
	if pr.ClosedAt != nil {
		closedAt := *pr.ClosedAt
		pr.ClosedAt = &closedAt
	}
	select {
	case d.queue <- item{pr: pr, event: event}:
	default:
		select {
		case d.overflow <- struct{}{}:
		default:
		}
	}
}

func (d *Dispatcher) run(ctx context.Context) {
	defer close(d.done)
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-d.overflow:
			if ctx.Err() != nil {
				return
			}
			if d.onFull != nil {
				d.onFull()
			}
		case item := <-d.queue:
			if ctx.Err() != nil {
				return
			}
			if d.deliver != nil {
				d.deliver(item.pr, item.event)
			}
		}
	}
}
