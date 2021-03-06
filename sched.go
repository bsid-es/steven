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
	Events() <-chan SchedEvent
	Reload(...*Event)
}

type sched struct {
	Now func() time.Time

	eventsC chan SchedEvent
	reloadC chan []*Event
}

func NewSched() (*sched, error) {
	return &sched{
		Now:     time.Now,
		eventsC: make(chan SchedEvent, 1),
		reloadC: make(chan []*Event, 1),
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

func (s *sched) Events() <-chan SchedEvent {
	return s.eventsC
}

func (s *sched) Reload(events ...*Event) {
	select {
	case <-s.reloadC:
	default:
	}
	s.reloadC <- events
}

func (s *sched) run(ctx context.Context) {
	q := make(schedQueue, 0)

	// We create a timer with the maximum duration possible so it won't fire
	// anytime soon, and its arm won't be selected on the next select.
	timer := time.NewTimer(1<<63 - 1)

	for {
		select {
		case <-ctx.Done():
			// Operation was canceled.
			timer.Stop()
			return

		case events := <-s.reloadC:
			// There are new events. Rebuild queue and schedule next event.
			seen := make(map[string]struct{}, len(events))
			q.clear()
			now := s.Now()
			for _, e := range events {
				if _, ok := seen[e.ID]; ok {
					continue
				}
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
				seen[e.ID] = struct{}{}
			}
			heap.Init(&q)

			if !timer.Stop() {
				<-timer.C
			}
			if len(q) > 0 {
				fire := &q[0]
				at := fire.at
				now := s.Now()
				timer.Reset(at.Sub(now))
			}

		case <-timer.C:
			// Fire event, and schedule next event.
			fire := &q[0]
			at := fire.at
			now := s.Now()
			if now.Before(at) {
				// Time drift. Sleep again.
				timer.Reset(at.Sub(now))
				continue
			}

			sevent := fire.sevent
			from := sevent.Current
			if sevent.Type != SchedEventStart || now.Before(sevent.end) {
				select {
				case <-ctx.Done():
					return
				case s.eventsC <- sevent:
				}
			} else {
				// Oops, we misfired... Pretend we fired the "stop" event, and
				// jump to the next instance closest to now.
				sevent.Type = SchedEventStop
				from = now
			}

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

			if len(q) > 0 {
				now := s.Now()
				at := fire.at
				timer.Reset(at.Sub(now))
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
