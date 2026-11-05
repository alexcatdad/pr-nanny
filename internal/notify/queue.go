// Package notify queues gated monitor events for the menu-bar app to read by cursor.
package notify

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/alexcatdad/pr-nanny/internal/core"
	"github.com/alexcatdad/pr-nanny/internal/model"
)

// MaxBatch bounds one NextNotifications reply and the retained queue.
const MaxBatch = 64

// Title returns the fixed notification title for an event, or "" for an
// unknown event. Titles never contain API data.
func Title(event core.Event) string {
	return map[core.Event]string{
		core.EventCI:              "CI status changed",
		core.EventApproval:        "Approval status changed",
		core.EventReviewRequested: "Review requested",
		core.EventMerged:          "Pull request merged",
		core.EventClosed:          "Pull request closed",
	}[event]
}

// DedupeID is a stable identifier for one observed PR state change, so a
// notification center can coalesce repeats of the same event.
func DedupeID(pr model.PullRequest, event core.Event) string {
	id := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s:%s:%t", pr.ID, event, pr.UpdatedAt.String(), pr.CI, pr.Approved)))
	return fmt.Sprintf("%x", id[:16])
}

// Notification is a gated notification ready for the UI process to post.
// Subtitle and Body are API text and must be rendered as plain text.
type Notification struct {
	ID       string `json:"id"`
	PRID     string `json:"prId"`
	Event    string `json:"event"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Body     string `json:"body"`
}

// Batch is one cursor-based read of the queue.
type Batch struct {
	Notifications []Notification `json:"notifications"`
	// Cursor is opaque; pass it to the next read.
	Cursor string `json:"cursor"`
	// Missed reports that older notifications were dropped before this read.
	Missed bool `json:"missed"`
}

type entry struct {
	seq   uint64
	pr    model.PullRequest
	event core.Event
	id    string
}

// Queue implements core.Notifier for a UI that pulls notifications. Notify
// never blocks the monitor; gating runs when the UI reads, so settings changed
// in between suppress queued items.
type Queue struct {
	mu      sync.Mutex
	epoch   string
	next    uint64
	items   []entry
	allowed func(string, core.Event) bool
}

// NewQueue returns an empty queue. allowed may be set later with SetFilter;
// until then nothing is delivered.
func NewQueue() *Queue {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return &Queue{epoch: fmt.Sprintf("%x", b), next: 1}
}

// SetFilter installs the delivery-time gate (normally Controller.NotificationAllowed).
func (q *Queue) SetFilter(allowed func(string, core.Event) bool) {
	q.mu.Lock()
	q.allowed = allowed
	q.mu.Unlock()
}

// Notify records an event, skipping unknown events and exact repeats still
// retained. The oldest entry is dropped when the bounded queue is full.
func (q *Queue) Notify(pr model.PullRequest, event core.Event) {
	if Title(event) == "" {
		return
	}
	id := DedupeID(pr, event)
	pr.ReviewRequestIDs = nil
	pr.UnverifiedTeamRequestIDs = nil
	pr.ClosedAt = nil
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, item := range q.items {
		if item.id == id {
			return
		}
	}
	if len(q.items) == MaxBatch {
		q.items = append(q.items[:0], q.items[1:]...)
	}
	q.items = append(q.items, entry{seq: q.next, pr: pr, event: event, id: id})
	q.next++
}

// Next returns gated notifications after cursor, at most limit (1..MaxBatch;
// other values mean MaxBatch). An empty, malformed or stale-epoch cursor reads
// from the oldest retained entry. Entries the gate rejects are consumed.
func (q *Queue) Next(cursor string, limit int) Batch {
	if limit <= 0 || limit > MaxBatch {
		limit = MaxBatch
	}
	q.mu.Lock()
	after, ok := q.parse(cursor)
	if !ok {
		after = 0
	}
	missed := ok && len(q.items) > 0 && q.items[0].seq > after+1
	var pending []entry
	last := after
	for _, item := range q.items {
		if item.seq <= after {
			continue
		}
		if len(pending) == limit {
			break
		}
		pending = append(pending, item)
		last = item.seq
	}
	if len(pending) == 0 && q.next-1 > last {
		last = q.next - 1
	}
	allowed := q.allowed
	epoch := q.epoch
	q.mu.Unlock()

	// The gate reads controller state; call it without holding the queue lock.
	batch := Batch{Notifications: []Notification{}, Cursor: epoch + "-" + strconv.FormatUint(last, 10), Missed: missed}
	for _, item := range pending {
		if allowed == nil || !allowed(item.pr.ID, item.event) {
			continue
		}
		batch.Notifications = append(batch.Notifications, Notification{
			ID: item.id, PRID: item.pr.ID, Event: string(item.event), Title: Title(item.event),
			Subtitle: fmt.Sprintf("%s #%d", item.pr.Repository, item.pr.Number), Body: item.pr.Title,
		})
	}
	return batch
}

func (q *Queue) parse(cursor string) (uint64, bool) {
	epoch, seq, found := strings.Cut(cursor, "-")
	if !found || epoch != q.epoch {
		return 0, false
	}
	n, err := strconv.ParseUint(seq, 10, 64)
	if err != nil || n >= q.next {
		return 0, false
	}
	return n, true
}
