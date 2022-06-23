package steven

import (
	"container/heap"
	"context"
	"errors"
	"time"
)

type SchedEventType int

const (
	SchedEventStart SchedEventType = iota
	SchedEventStop
)

type SchedEvent struct {
	Type          SchedEventType
	Event         *Event
	Current, Next time.Time
	end           time.Time
}

type Sched interface {
	Run(context.Context) error
	Subscribe(context.Context, chan<- SchedEvent)
	Unsubscribe(context.Context, chan<- SchedEvent)
	Reload(...*Event)
}

type sched struct {
	Now func() time.Time

	newEvents chan []*Event

	subscribe   chan chan<- SchedEvent
	unsubscribe chan chan<- SchedEvent
}

func NewSched() (*sched, error) {
	return &sched{
		Now:       time.Now,
		newEvents: make(chan []*Event, 1),

		subscribe:   make(chan chan<- SchedEvent),
		unsubscribe: make(chan chan<- SchedEvent),
	}, nil
}

var _ Sched = (*sched)(nil)

func (s *sched) Run(ctx context.Context) error {
	if s.Now == nil {
		return errors.New("need a Now function")
	}
	go s.run(ctx)
	return nil
}

func (s *sched) Subscribe(ctx context.Context, sub chan<- SchedEvent) {
	s.subscribe <- sub
}

func (s *sched) Unsubscribe(ctx context.Context, sub chan<- SchedEvent) {
	s.unsubscribe <- sub
}

func (s *sched) Reload(newEvents ...*Event) {
	select {
	case <-s.newEvents:
	default:
	}
	s.newEvents <- newEvents
}

func (s *sched) run(ctx context.Context) {
	subs := make(map[chan<- SchedEvent]struct{})
	defer func() {
		for sub := range subs {
			close(sub)
		}
	}()

	q := make(schedQueue, 0)
	timer := time.NewTimer(1<<63 - 1)
	send := make(chan SchedEvent, 1)
	for {
		select {
		case <-ctx.Done():
			timer.Stop()
			return

		case sub := <-s.subscribe:
			subs[sub] = struct{}{}

		case sub := <-s.unsubscribe:
			delete(subs, sub)

		case events := <-s.newEvents:
			// Rebuild queue.
			q.clear()
			now := s.Now()
			for _, e := range events {
				if curr := e.Current(now); !curr.IsZero() {
					end := curr.Add(e.Duration)
					q = append(q, schedQueueEntry{
						at: end,
						sevent: SchedEvent{
							Type:    SchedEventStop,
							Event:   e,
							Current: curr,
							Next:    e.Next(curr),
							end:     end,
						},
					})
				} else if next := e.Next(now); !next.IsZero() {
					end := next.Add(e.Duration)
					q = append(q, schedQueueEntry{
						at: next,
						sevent: SchedEvent{
							Type:    SchedEventStart,
							Event:   e,
							Current: next,
							Next:    e.Next(next),
							end:     end,
						},
					})
				}
			}
			heap.Init(&q)

			// Schedule next event.
			if !timer.Stop() {
				<-timer.C
			}
			if len(q) > 0 {
				next := &q[0]
				at := next.at
				now := s.Now()
				timer.Reset(at.Sub(now))
			}

		case <-timer.C:
			// Get next event to fire.
			fire := &q[0]
			at := fire.at
			now := s.Now()
			if now.Before(at) {
				timer.Reset(at.Sub(now))
				continue
			}

			// Send event.
			sevent := fire.sevent
			from := sevent.Current
			if sevent.Type != SchedEventStart || now.Before(sevent.end) {
				send <- sevent
			} else {
				// There was a misfire. Pretend we fired the "stop" event,
				// and jump to the next instance closest to now.
				sevent.Type = SchedEventStop
				from = now
			}

			// Reschedule event.
			event := sevent.Event
			if sevent.Type == SchedEventStart {
				// Schedule event stop.
				fire.at = sevent.end
				fire.sevent.Type = SchedEventStop
				heap.Fix(&q, 0)
			} else if next := event.Next(from); !next.IsZero() {
				// There's another instance to run. Schedule it.
				fire.at = next
				fire.sevent.Type = SchedEventStart
				fire.sevent.Current = next
				fire.sevent.Next = event.Next(next)
				fire.sevent.end = next.Add(event.Duration)
				heap.Fix(&q, 0)
			} else {
				// Event is finished. Drop it.
				heap.Pop(&q)
			}

			// Sleep until next event.
			if len(q) > 0 {
				now := s.Now()
				at := fire.at
				timer.Reset(at.Sub(now))
			}

		case sevent := <-send:
			for sub := range subs {
				select {
				case <-ctx.Done():
					return
				case sub <- sevent:
				}
			}
		}
	}
}

type schedQueueEntry struct {
	at     time.Time
	sevent SchedEvent
}

type schedQueue []schedQueueEntry

var _ heap.Interface = (*schedQueue)(nil)

func (q schedQueue) Len() int {
	return len(q)
}

func (q schedQueue) Less(i, j int) bool {
	ti, tj := q[i].at, q[j].at
	return ti.Before(tj)
}

func (q schedQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}

func (q *schedQueue) Push(x any) {
	*q = append(*q, x.(schedQueueEntry))
}

func (q *schedQueue) Pop() any {
	old := *q
	n := len(old)
	it := old[n-1]
	old[n-1] = schedQueueEntry{}
	*q = old[:n-1]
	return it
}

func (q *schedQueue) clear() {
	for i := range *q {
		(*q)[i] = schedQueueEntry{}
	}
	*q = (*q)[:0]
}
