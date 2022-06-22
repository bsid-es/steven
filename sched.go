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
	reload    chan struct{}

	mu   sync.Mutex
	subs map[chan<- SchedEvent]struct{}
}

func NewSched() (*sched, error) {
	return &sched{
		Now:       time.Now,
		newEvents: make(chan []*Event, 1),
		reload:    make(chan struct{}, 1),
		subs:      make(map[chan<- SchedEvent]struct{}),
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs[sub] = struct{}{}
}

func (s *sched) Unsubscribe(ctx context.Context, sub chan<- SchedEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, sub)
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
		for sub := range s.subs {
			close(sub)
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

		default:
			// Get next event.
			if len(q) == 0 {
				select {
				case <-ctx.Done():
					return
				case <-s.reload:
					continue again
				}
			}
			at, sevent := q[0].at, q[0].sevent
			event := sevent.Event

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
			from := sevent.Current
			if sevent.Type != SchedEventStart || now.Before(sevent.end) {
				s.mu.Lock()
				for sub := range s.subs {
					select {
					case <-ctx.Done():
						s.mu.Unlock()
						return
					case <-s.reload:
						s.mu.Unlock()
						continue again
					case sub <- sevent:
					}
				}
				s.mu.Unlock()
			} else {
				// There was a misfire. Pretend we fired the "stop" event,
				// and jump to the next instance closest to now.
				sevent.Type = SchedEventStop
				from = now
			}

			// Reschedule event.
			if sevent.Type == SchedEventStart {
				// Schedule event stop.
				entry := &q[0]
				entry.at = sevent.end
				entry.sevent.Type = SchedEventStop
				heap.Fix(&q, 0)
			} else if next := event.Next(from); !next.IsZero() {
				// There's another instance to run. Reschedule event.
				entry := &q[0]
				entry.at = next
				entry.sevent.Type = SchedEventStart
				entry.sevent.Current = next
				entry.sevent.Next = event.Next(next)
				entry.sevent.end = next.Add(event.Duration)
				heap.Fix(&q, 0)
			} else {
				// Event is finished. Drop it.
				heap.Pop(&q)
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
