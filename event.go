package steven

import (
	"encoding/json"
	"errors"
	"time"
)

type Event struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Start      time.Time     `json:"start"`
	Duration   time.Duration `json:"duration"`
	Interval   time.Duration `json:"interval,omitempty"`
	LimitUntil time.Time     `json:"limitUntil,omitempty"`
	LimitCount uint          `json:"limitCount,omitempty"`

	last time.Time
}

var _ json.Unmarshaler = (*Event)(nil)

func (e *Event) UnmarshalJSON(data []byte) (err error) {
	type event2 Event
	if err = json.Unmarshal(data, (*event2)(e)); err == nil {
		err = e.Validate()
	}
	return err
}

func (e *Event) Validate() (err error) {
	switch {
	case e.Duration <= 0:
		return errors.New("duration must be non-negative")
	case e.Interval < 0:
		return errors.New("interval must be non-negative")
	case e.Interval > 0 && e.Interval < e.Duration:
		return errors.New("interval must be no less than duration")
	case e.Interval > 0 && !e.LimitUntil.IsZero() && e.LimitCount != 0:
		return errors.New("limitUntil and limitCount are mutually exclusive")
	case e.Interval > 0 && !e.LimitUntil.IsZero() && !e.LimitUntil.After(e.Start):
		return errors.New("limitUntil must happen after start")
	}
	switch {
	case e.Interval == 0:
		e.last = e.Start
	case e.Interval > 0 && !e.LimitUntil.IsZero():
		count := e.LimitUntil.Sub(e.Start) / e.Interval
		e.last = e.Start.Add(count * e.Interval)
	case e.Interval > 0 && e.LimitCount != 0:
		count := time.Duration(e.LimitCount) - 1
		e.last = e.Start.Add(count * e.Interval)
	}
	return nil
}

func (e *Event) Previous(from time.Time) time.Time {
	switch {
	case from.Sub(e.Start) < e.Duration:
		// Event didn't start yet, or the current instance is still running.
		return time.Time{}
	case !e.last.IsZero() && from.Sub(e.last) >= e.Duration:
		// Event is finished.
		return e.last.In(from.Location())
	}
	inst := e.instance(from)
	if from.Sub(inst) < e.Duration {
		// Instance is still running; get the second to previous instance.
		return inst.Add(-e.Interval)
	}
	return inst
}

func (e *Event) Current(from time.Time) time.Time {
	switch {
	case from.Before(e.Start):
		// Event didn't start yet.
		return time.Time{}
	case from.Sub(e.Start) < e.Duration:
		// Current instance is running.
		return e.Start.In(from.Location())
	case !e.last.IsZero() && from.Sub(e.last) >= e.Duration:
		// Event is finished.
		return time.Time{}
	}
	inst := e.instance(from)
	if from.Sub(inst) >= e.Duration {
		// Instance is finished.
		return time.Time{}
	}
	return inst
}

func (e *Event) Next(from time.Time) time.Time {
	switch {
	case from.Before(e.Start):
		// Event didn't start yet.
		return e.Start.In(from.Location())
	case !e.last.IsZero() && !from.Before(e.last):
		// Event is finished.
		return time.Time{}
	}
	return e.instance(from.Add(e.Interval))
}

func (e *Event) instance(from time.Time) time.Time {
	count := from.Sub(e.Start) / e.Interval
	return e.Start.Add(count * e.Interval).In(from.Location())
}
