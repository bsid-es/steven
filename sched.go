package steven

import (
	"container/heap"
	"context"
	"errors"
	"sync"
	"time"
)

type SchedEventType int

const (
	SchedEventStart SchedEventType = iota
	SchedEventStop
)

type SchedEvent struct {
	Type       SchedEventType
	Event      *Event
	Start, End time.Time
}

type Sched interface {
	Run(context.Context) error
	Register(chan<- SchedEvent)
	Reload(...*Event)
}

type sched struct {
	Now func() time.Time

	newEvents chan []*Event
	reload    chan struct{}

	mu        sync.Mutex
	listeners []chan<- SchedEvent
}

func NewSched() *sched {
	return &sched{
		Now:       time.Now,
		newEvents: make(chan []*Event, 1),
		reload:    make(chan struct{}, 1),
	}
}

var _ Sched = (*sched)(nil)

func (s *sched) Run(ctx context.Context) error {
	if s.Now == nil {
		return errors.New("need a Now function")
	}
	go s.run(ctx)
	return nil
}

func (s *sched) Register(listener chan<- SchedEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, listener)
}

func (s *sched) Reload(newEvents ...*Event) {
	select {
	case <-s.newEvents:
	default:
	}
	s.newEvents <- newEvents

	select {
	case s.reload <- struct{}{}:
	default:
	}
}

func (s *sched) run(ctx context.Context) {
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, ln := range s.listeners {
			close(ln)
		}
	}()

	q := make(schedQueue, 0)
again:
	for {
		select {
		case <-ctx.Done():
			return

		case events := <-s.newEvents:
			// Rebuild queue.
			q.clear()
			now := s.Now()
			for _, e := range events {
				if curr := e.Current(now); !curr.IsZero() {
					end := curr.Add(e.Duration)
					q = append(q, schedQueueEntry{
						at: end,
						event: SchedEvent{
							Type:  SchedEventStop,
							Event: e,
							Start: curr,
							End:   end,
						},
					})
				} else if next := e.Next(now); !next.IsZero() {
					q = append(q, schedQueueEntry{
						at: next,
						event: SchedEvent{
							Type:  SchedEventStart,
							Event: e,
							Start: next,
							End:   next.Add(e.Duration),
						},
					})
				}
			}
			heap.Init(&q)

		default:
		}

		// Get next event.
		if len(q) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-s.reload:
				continue again
			}
		}
		at, event := q[0].at, q[0].event

		// Sleep until event fires.
		now := s.Now()
		for now.Before(at) {
			timer := time.NewTimer(at.Sub(now))
			stop := func() {
				if !timer.Stop() {
					<-timer.C
				}
			}
			select {
			case <-ctx.Done():
				stop()
				return
			case <-s.reload:
				stop()
				continue again
			case <-timer.C:
			}
			now = s.Now()
		}

		// Send event.
		from := event.Start
		if event.Type != SchedEventStart || now.Before(event.End) {
			s.mu.Lock()
			for _, ln := range s.listeners {
				select {
				case <-ctx.Done():
					s.mu.Unlock()
					return
				case <-s.reload:
					s.mu.Unlock()
					continue again
				case ln <- event:
				}
			}
			s.mu.Unlock()
		} else {
			// There was a misfire. Pretend we fired the "stop" event,
			// and jump to the next instance closest to now.
			event.Type = SchedEventStop
			from = s.Now()
		}

		// Reschedule event.
		if event.Type == SchedEventStart {
			// Schedule event stop.
			q[0] = schedQueueEntry{
				at: event.End,
				event: SchedEvent{
					Type:  SchedEventStop,
					Event: event.Event,
					Start: event.Start,
					End:   event.End,
				},
			}
			heap.Fix(&q, 0)
		} else if next := event.Event.Next(from); !next.IsZero() {
			// There's another instance to run. Reschedule event.
			q[0] = schedQueueEntry{
				at: next,
				event: SchedEvent{
					Type:  SchedEventStart,
					Event: event.Event,
					Start: next,
					End:   next.Add(event.Event.Duration),
				},
			}
			heap.Fix(&q, 0)
		} else {
			// Event is finished. Drop it.
			heap.Pop(&q)
		}
	}
}

type schedQueueEntry struct {
	at    time.Time
	event SchedEvent
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
