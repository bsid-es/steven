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
	Event         *Event
	Current, Next time.Time
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

	newEvents   chan []*Event
	subscribe   chan chan<- BusEvent
	unsubscribe chan chan<- BusEvent
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
	schedC := b.sched.Events()
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
			for id := range entries {
				delete(entries, id)
			}
			now := b.Now()
			for _, event := range events {
				curr := event.Current(now)
				next := event.Next(now)
				if curr.IsZero() && next.IsZero() {
					continue
				}
				entries[event.ID] = BusEntry{
					Event:   event,
					Current: curr,
					Next:    next,
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

		case sevent := <-schedC:
			if sevent.Event == nil {
				continue
			}
			id := sevent.Event.ID
			entry := entries[id]
			entry.Current = sevent.Current
			entry.Next = sevent.Next

			var typ BusEventType
			switch sevent.Type {
			case SchedEventStart:
				typ = BusEventStart
			case SchedEventStop:
				typ = BusEventStop
			}

			single := make(map[string]BusEntry, 1)
			single[id] = entry
			bevent := BusEvent{
				Type:    typ,
				Entries: single,
			}
			for sub := range subs {
				select {
				case <-ctx.Done():
					return
				case sub <- bevent:
				}
			}

			if typ == BusEventStop && entry.Next.IsZero() {
				delete(entries, id)
			} else {
				entries[id] = entry
			}
		}
	}
}
