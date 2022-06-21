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

type Scheduler interface {
	Reload(...*Event)
	Run(context.Context) error
	Events() <-chan SchedEvent
}

type scheduler struct {
	Now func() time.Time

	reloadC chan []*Event

	mu     sync.Mutex
	q      schedQueue
	againC chan struct{}

	eventsC chan SchedEvent
}

func NewScheduler() *scheduler {
	return &scheduler{
		Now:     time.Now,
		reloadC: make(chan []*Event, 1),
		againC:  make(chan struct{}, 1),
		eventsC: make(chan SchedEvent),
	}
}

var _ Scheduler = (*scheduler)(nil)

func (s *scheduler) Reload(events ...*Event) {
	select {
	case <-s.reloadC:
	default:
	}
	s.reloadC <- events
}

func (s *scheduler) Run(ctx context.Context) error {
	if s.Now == nil {
		return errors.New("need a Now function")
	}

	go func() {
		<-ctx.Done()
		close(s.eventsC)
	}()
	go s.reload(ctx)
	go s.run(ctx)

	return nil
}

// Events must be called after Run.
func (s *scheduler) Events() <-chan SchedEvent {
	return s.eventsC
}

func (s *scheduler) reload(ctx context.Context) {
	mu, q, again := &s.mu, &s.q, s.againC
	for {
		select {
		case <-ctx.Done():
			return

		case events := <-s.reloadC:
			mu.Lock()

			// Rebuild queue.
			q.clear()
			now := s.Now()
			for _, e := range events {
				if curr := e.Current(now); !curr.IsZero() {
					end := curr.Add(e.Duration)
					*q = append(*q, schedQueueEntry{
						at: end,
						event: SchedEvent{
							Type:  SchedEventStop,
							Event: e,
							Start: curr,
							End:   end,
						},
					})
				} else if next := e.Next(now); !next.IsZero() {
					*q = append(*q, schedQueueEntry{
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
			heap.Init(q)

			// Emit signal to restart scheduling.
			select {
			case again <- struct{}{}:
			default:
			}

			mu.Unlock()
		}
	}
}

// TODO(emerson):
//  - Handle backpressure.
func (s *scheduler) run(ctx context.Context) {
	mu, q, again := &s.mu, &s.q, s.againC
again:
	for {
		// Get next event.
		mu.Lock()
		if len(*q) == 0 {
			mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-again:
				continue again
			}
		}
		at, event := (*q)[0].at, (*q)[0].event
		mu.Unlock()

		// Sleep until event fires.
		now := s.Now()
		for now.Before(at) {
			sleep := time.NewTimer(at.Sub(now))
			stopSleep := func() {
				if !sleep.Stop() {
					<-sleep.C
				}
			}
			select {
			case <-ctx.Done():
				stopSleep()
				return
			case <-again:
				stopSleep()
				continue again
			case <-sleep.C:
			}
			now = s.Now()
		}

		// Send event.
		from := event.Start
		if event.Type != SchedEventStart || now.Before(event.End) {
			select {
			case <-ctx.Done():
				return
			case <-again:
				continue again
			case s.eventsC <- event:
			}
		} else {
			// There was a misfire.
			event.Type = SchedEventStop
			from = s.Now()
		}

		// Reschedule event.
		mu.Lock()
		select {
		case <-again:
			mu.Unlock()
			continue again
		default:
		}
		if event.Type == SchedEventStart {
			// Schedule event stop.
			(*q)[0] = schedQueueEntry{
				at: event.End,
				event: SchedEvent{
					Type:  SchedEventStop,
					Event: event.Event,
					Start: event.Start,
					End:   event.End,
				},
			}
			heap.Fix(q, 0)
		} else if next := event.Event.Next(from); !next.IsZero() {
			// There's another instance to run. Reschedule event.
			(*q)[0] = schedQueueEntry{
				at: next,
				event: SchedEvent{
					Type:  SchedEventStart,
					Event: event.Event,
					Start: next,
					End:   next.Add(event.Event.Duration),
				},
			}
			heap.Fix(q, 0)
		} else {
			// Event is finished. Drop it.
			heap.Pop(q)
		}
		mu.Unlock()
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
