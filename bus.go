package steven

import (
	"context"
	"errors"
	"sync"
	"time"
)

type BusEventType int

const (
	BusEventReset BusEventType = iota
	BusEventStart
	BusEventStop
)

type BusEvent struct {
	Type    BusEventType
	Entries []BusEntry
}

type BusEntry struct {
	Event   *Event
	Current time.Time
	Next    time.Time
}

type Bus interface {
	Run(context.Context) error
	Events() <-chan BusEvent
	Reload(...*Event)
	Entries() []BusEntry
}

type bus struct {
	Now func() time.Time

	sched Sched

	eventsC chan BusEvent
	reloadC chan []*Event

	mu      sync.RWMutex
	entries map[string]BusEntry
}

func NewBus(sched Sched) (*bus, error) {
	if sched == nil {
		return nil, errors.New("nil sched")
	}
	return &bus{
		Now:     time.Now,
		sched:   sched,
		eventsC: make(chan BusEvent, 1),
		reloadC: make(chan []*Event, 1),
		entries: make(map[string]BusEntry),
	}, nil
}

var _ Bus = (*bus)(nil)

func (b *bus) Run(ctx context.Context) error {
	if b.Now == nil {
		return errors.New("need a Now function")
	}
	go b.run(ctx)
	return nil
}

func (b *bus) Events() <-chan BusEvent {
	return b.eventsC
}

func (b *bus) Reload(events ...*Event) {
	select {
	case <-b.reloadC:
	default:
	}
	b.reloadC <- events
}

func (b *bus) Entries() []BusEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	entries := make([]BusEntry, 0, len(b.entries))
	for _, entry := range b.entries {
		entries = append(entries, entry)
	}
	return entries
}

func (b *bus) run(ctx context.Context) {
	schedC := b.sched.Events()
	for {
		select {
		case <-ctx.Done():
			return

		case events := <-b.reloadC:
			// Rebuild event map.
			b.mu.Lock()
			for id := range b.entries {
				delete(b.entries, id)
			}
			now := b.Now()
			entries := make([]BusEntry, 0, len(events))
			for _, event := range events {
				curr := event.Current(now)
				next := event.Next(now)
				if curr.IsZero() && next.IsZero() {
					continue
				}
				entry := BusEntry{
					Event:   event,
					Current: curr,
					Next:    next,
				}
				entries = append(entries, entry)
				b.entries[event.ID] = entry
			}
			b.mu.Unlock()

			// Send RESET event.
			bevent := BusEvent{
				Type:    BusEventReset,
				Entries: entries,
			}
			select {
			case <-ctx.Done():
				return
			case b.eventsC <- bevent:
			}

		case sevent := <-schedC:
			if sevent.Event == nil {
				continue
			}
			id := sevent.Event.ID
			entry := b.entries[id]
			entry.Current = sevent.Current
			entry.Next = sevent.Next

			var typ BusEventType
			switch sevent.Type {
			case SchedEventStart:
				typ = BusEventStart
			case SchedEventStop:
				typ = BusEventStop
			}

			var single [1]BusEntry
			single[0] = entry
			bevent := BusEvent{
				Type:    typ,
				Entries: single[:],
			}
			select {
			case <-ctx.Done():
				return
			case b.eventsC <- bevent:
			}

			b.mu.Lock()
			if typ == BusEventStop && entry.Next.IsZero() {
				delete(b.entries, id)
			} else {
				b.entries[id] = entry
			}
			b.mu.Unlock()
		}
	}
}
