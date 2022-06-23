package steven

import (
	"context"
	"errors"
	"sync"
	"time"
)

type BusSync struct {
	Events []BusEvent `json:"events"`
}

type BusEvent struct {
	Event   *Event    `json:"event"`
	Current time.Time `json:"current"`
	Next    time.Time `json:"next"`
}

type Bus interface {
	Run(context.Context) error
	Register(context.Context, chan<- BusSync) func()
	Reload(...*Event)
}

type bus struct {
	Now func() time.Time

	sched Sched

	reloadC chan []*Event

	clientsMtx sync.Mutex
	clients    map[chan<- BusSync]struct{}

	eventsMtx sync.RWMutex
	events    map[string]BusEvent
}

func NewBus(sched Sched) (*bus, error) {
	if sched == nil {
		return nil, errors.New("nil sched")
	}
	return &bus{
		Now:     time.Now,
		sched:   sched,
		reloadC: make(chan []*Event, 1),
		clients: make(map[chan<- BusSync]struct{}),
		events:  make(map[string]BusEvent),
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

func (b *bus) Register(ctx context.Context, client chan<- BusSync) func() {
	go b.register(ctx, client)
	return func() {
		b.clientsMtx.Lock()
		defer b.clientsMtx.Unlock()
		delete(b.clients, client)
	}
}

func (b *bus) register(ctx context.Context, client chan<- BusSync) {
	b.clientsMtx.Lock()
	defer b.clientsMtx.Unlock()
	b.clients[client] = struct{}{}

	b.eventsMtx.RLock()
	events := make([]BusEvent, 0, len(b.events))
	for _, event := range b.events {
		events = append(events, event)
	}
	b.eventsMtx.RUnlock()

	bsync := BusSync{Events: events}
	select {
	case <-ctx.Done():
	case client <- bsync:
	}
}

func (b *bus) Reload(events ...*Event) {
	select {
	case <-b.reloadC:
	default:
	}
	b.reloadC <- events
}

func (b *bus) run(ctx context.Context) {
	schedC := b.sched.Events()
	for {
		select {
		case <-ctx.Done():
			return

		case newEvents := <-b.reloadC:
			// Rebuild event map.
			// TODO(fmrsn): Make diff instead of discarding entire map.
			b.eventsMtx.Lock()
			for id := range b.events {
				delete(b.events, id)
			}
			now := b.Now()
			events := make([]BusEvent, 0, len(newEvents))
			for _, event := range newEvents {
				curr := event.Current(now)
				next := event.Next(now)
				if curr.IsZero() && next.IsZero() {
					continue
				}
				entry := BusEvent{
					Event:   event,
					Current: curr,
					Next:    next,
				}
				events = append(events, entry)
				b.events[event.ID] = entry
			}
			b.eventsMtx.Unlock()

			// Send sync event.
			b.clientsMtx.Lock()
			bsync := BusSync{Events: events}
			for c := range b.clients {
				select {
				case <-ctx.Done():
					return
				case c <- bsync:
				}
			}
			b.clientsMtx.Unlock()

		case sevent := <-schedC:
			if sevent.Event == nil {
				continue
			}
			id := sevent.Event.ID
			entry := b.events[id]
			switch sevent.Type {
			case SchedEventStart:
				entry.Current = sevent.Current
			case SchedEventStop:
				entry.Current = time.Time{}
			}
			entry.Next = sevent.Next

			b.clientsMtx.Lock()
			bsync := BusSync{Events: []BusEvent{entry}}
			for c := range b.clients {
				select {
				case <-ctx.Done():
					return
				case c <- bsync:
				}
			}
			b.clientsMtx.Unlock()

			b.eventsMtx.Lock()
			if entry.Current.IsZero() && entry.Next.IsZero() {
				delete(b.events, id)
			} else {
				b.events[id] = entry
			}
			b.eventsMtx.Unlock()
		}
	}
}
