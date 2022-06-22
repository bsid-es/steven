package steven

import (
	"context"
	"errors"
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
	Entries map[string]BusEntry
}

type BusEntry struct {
	Event                   *Event
	Previous, Current, Next time.Time
}

type Bus interface {
	Run(context.Context) error
	Subscribe(context.Context, chan<- BusEvent)
	Unsubscribe(context.Context, chan<- BusEvent)
	Reload(...*Event)
}

type bus struct {
	Now func() time.Time

	sched Sched

	newEvents chan []*Event

	subscribe   chan chan<- BusEvent
	unsubscribe chan chan<- BusEvent

	schedEvents chan SchedEvent
}

func NewBus(sched Sched) (*bus, error) {
	if sched == nil {
		return nil, errors.New("nil sched")
	}
	return &bus{
		Now:         time.Now,
		sched:       sched,
		newEvents:   make(chan []*Event, 1),
		subscribe:   make(chan chan<- BusEvent),
		unsubscribe: make(chan chan<- BusEvent),
		schedEvents: make(chan SchedEvent, 1),
	}, nil
}

var _ Bus = (*bus)(nil)

func (b *bus) Run(ctx context.Context) error {
	if b.Now == nil {
		return errors.New("need a Now function")
	}
	b.sched.Subscribe(ctx, b.schedEvents)
	go b.run(ctx)
	return nil
}

func (b *bus) Subscribe(ctx context.Context, sub chan<- BusEvent) {
	select {
	case <-ctx.Done():
	case b.subscribe <- sub:
	}
}

func (b *bus) Unsubscribe(ctx context.Context, sub chan<- BusEvent) {
	select {
	case <-ctx.Done():
	case b.unsubscribe <- sub:
	}
}

func (b *bus) Reload(events ...*Event) {
	select {
	case <-b.newEvents:
	default:
	}
	b.newEvents <- events
}

func (b *bus) run(ctx context.Context) {
	subs := make(map[chan<- BusEvent]struct{})
	defer func() {
		for ln := range subs {
			close(ln)
		}
	}()

	entries := make(map[string]BusEntry)
	for {
		select {
		case <-ctx.Done():
			return

		case sub := <-b.subscribe:
			subs[sub] = struct{}{}
			bevent := BusEvent{
				Type:    BusEventReset,
				Entries: entries,
			}
			select {
			case <-ctx.Done():
				return
			case sub <- bevent:
			}

		case sub := <-b.unsubscribe:
			delete(subs, sub)

		case events := <-b.newEvents:
			// Rebuild event map.
			for k := range entries {
				delete(entries, k)
			}
			now := b.Now()
			for _, event := range events {
				entries[event.ID] = BusEntry{
					Event:    event,
					Previous: event.Previous(now),
					Current:  event.Current(now),
					Next:     event.Next(now),
				}
			}

			// Send RESET event.
			bevent := BusEvent{
				Type:    BusEventReset,
				Entries: entries,
			}
			for sub := range subs {
				select {
				case <-ctx.Done():
					return
				case sub <- bevent:
				}
			}

		case sevent := <-b.schedEvents: // TODO(fmrsn): Rename.
			var typ BusEventType

			k := sevent.Event.ID
			entry := entries[k]
			switch sevent.Type {
			case SchedEventStart:
				typ = BusEventStart
				entry.Current = sevent.Current
				entry.Next = sevent.Next
			case SchedEventStop:
				typ = BusEventStop
				entry.Previous = sevent.Current
				entry.Current = time.Time{}
				entry.Next = sevent.Next
			}
			entries[k] = entry

			e := make(map[string]BusEntry, 1)
			e[k] = entry
			bevent := BusEvent{
				Type:    typ,
				Entries: e,
			}
			for sub := range subs {
				select {
				case <-ctx.Done():
					return
				case sub <- bevent:
				}
			}
		}
	}
}
