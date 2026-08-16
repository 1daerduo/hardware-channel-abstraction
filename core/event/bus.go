// Package event provides the in-memory EventBus and EventStore (Design doc
// 08). Delivery is at-least-once; consumers dedupe by event ID. Sequence is
// monotonically increasing and supports cursor-based resume.
package event

import (
	"context"
	"fmt"
	"sync"

	"example.com/embedded-loop-channel/domain"
)

// Filter selects events for a subscription. Zero-valued fields match all.
type Filter struct {
	DeviceID    domain.DeviceID
	OperationID domain.OperationID
	Type        string
}

// matches reports whether an event passes the filter.
func (f Filter) matches(e domain.Event) bool {
	if f.DeviceID != "" && e.DeviceID != f.DeviceID {
		return false
	}
	if f.OperationID != "" && e.OperationID != f.OperationID {
		return false
	}
	if f.Type != "" && e.Type != f.Type {
		return false
	}
	return true
}

// Subscription delivers a stream of matching events.
type Subscription struct {
	C      <-chan domain.Event
	cancel func()
}

// Close stops the subscription.
func (s *Subscription) Close() { s.cancel() }

// Bus is a publish/subscribe event bus with an append-only store.
type Bus struct {
	mu     sync.Mutex
	seq    uint64
	events []domain.Event
	subs   map[int]subscriber
	nextID int
}

type subscriber struct {
	ch     chan domain.Event
	filter Filter
}

// New builds an empty Bus.
func New() *Bus {
	return &Bus{subs: map[int]subscriber{}}
}

// Publish assigns a sequence, appends to the store and fans out to matching
// subscribers. The event ID must be set by the caller.
func (b *Bus) Publish(e *domain.Event) {
	b.mu.Lock()
	b.seq++
	e.Sequence = b.seq
	b.events = append(b.events, *e)

	// Snapshot subscribers; delivery happens after unlock to avoid a slow
	// consumer blocking producers.
	matched := make([]subscriber, 0, len(b.subs))
	for _, s := range b.subs {
		if s.filter.matches(*e) {
			matched = append(matched, s)
		}
	}
	b.mu.Unlock()

	for _, s := range matched {
		select {
		case s.ch <- *e:
		default: // drop if the subscriber is not draining (backpressure)
		}
	}
}

// Subscribe registers a subscription for events matching filter. The channel
// is buffered; consumers must drain it or events are dropped (backpressure).
func (b *Bus) Subscribe(filter Filter) *Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.nextID
	b.nextID++
	ch := make(chan domain.Event, 64)
	b.subs[id] = subscriber{ch: ch, filter: filter}
	return &Subscription{
		C: ch,
		cancel: func() {
			b.mu.Lock()
			delete(b.subs, id)
			b.mu.Unlock()
		},
	}
}

// Events returns stored events matching the filter, ordered by sequence.
func (b *Bus) Events(filter Filter) []domain.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []domain.Event
	for _, e := range b.events {
		if filter.matches(e) {
			out = append(out, e)
		}
	}
	return out
}

// EventsAfter returns stored events with sequence > cursor, matching filter.
func (b *Bus) EventsAfter(filter Filter, cursor uint64) []domain.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []domain.Event
	for _, e := range b.events {
		if e.Sequence > cursor && filter.matches(e) {
			out = append(out, e)
		}
	}
	return out
}

// NewEvent builds an event and publishes it (helper for Core layers).
func (b *Bus) Emit(typ, producer, subject string) *domain.Event {
	e := domain.NewEvent(typ, producer, subject)
	b.Publish(e)
	return e
}

// Waitable helper for tests: drain a subscription into a slice until ctx is
// done or n events collected.
func Drain(ctx context.Context, sub *Subscription, n int) []domain.Event {
	var out []domain.Event
	for len(out) < n {
		select {
		case e, ok := <-sub.C:
			if !ok {
				return out
			}
			out = append(out, e)
		case <-ctx.Done():
			return out
		}
	}
	return out
}

// String renders an event for debugging.
func String(e domain.Event) string {
	return fmt.Sprintf("#%d %s (%s)", e.Sequence, e.Type, e.Subject)
}
