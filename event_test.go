package steven_test

import (
	"encoding/json"
	"testing"
	"time"

	"bsid.es/steven"
)

var validationTests = []struct {
	name  string
	event steven.Event
}{{
	name: "negative duration",
	event: steven.Event{
		Duration: -1,
	},
}, {
	name: "zero duration",
	event: steven.Event{
		Duration: 0,
	},
}, {
	name: "negative interval between instances",
	event: steven.Event{
		Duration: 1 * time.Minute,
		Interval: -1,
	},
}, {
	name: "duration higher than interval between instances",
	event: steven.Event{
		Duration: 2 * time.Minute,
		Interval: 1 * time.Minute,
	},
}, {
	name: "limituntil and limitcount set simultaneously",
	event: steven.Event{
		Duration:   1 * time.Minute,
		Interval:   1 * time.Minute,
		LimitUntil: time.Now(),
		LimitCount: 1,
	},
}, {
	name: "limituntil before start",
	event: steven.Event{
		Start:      time.Date(2021, 12, 21, 0, 0, 0, 0, time.UTC),
		Duration:   1 * time.Minute,
		Interval:   1 * time.Minute,
		LimitUntil: time.Date(2021, 12, 20, 0, 0, 0, 0, time.UTC),
	},
}, {
	name: "same start and limituntil",
	event: steven.Event{
		Start:      time.Date(2021, 12, 21, 0, 0, 0, 0, time.UTC),
		Duration:   1 * time.Minute,
		Interval:   1 * time.Minute,
		LimitUntil: time.Date(2021, 12, 21, 0, 0, 0, 0, time.UTC),
	},
}}

func TestEventUnmarshalJSON(t *testing.T) {
	for _, tt := range validationTests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.event)
			if err != nil {
				t.Fatal(err)
			}
			var dummy steven.Event
			if err := json.Unmarshal(encoded, &dummy); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestEventValidate(t *testing.T) {
	for _, tt := range validationTests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.event.Validate(); err == nil {
				t.Error("expected error")
			}
		})
	}
}

var (
	refStart       = time.Date(2012, 12, 21, 0, 0, 0, 0, time.UTC)
	refDuration    = 10 * time.Minute
	refInterval    = 20 * time.Minute
	refFirstStart  = refStart
	refFirstEnd    = refStart.Add(refDuration)
	refSecondStart = refStart.Add(refInterval)
	refSecondEnd   = refSecondStart.Add(refDuration)
	refThirdStart  = refSecondStart.Add(refInterval)
)

func TestEventInstances(t *testing.T) {
	type params struct {
		name                   string
		from, last, curr, next time.Time
	}
	tests := []struct {
		name   string
		event  steven.Event
		params []params
	}{{
		name: "event with single instance",
		event: steven.Event{
			Start:    refStart,
			Duration: refDuration,
		},
		params: []params{{
			name: "before single instance",
			from: refFirstStart.Add(-1),
			last: time.Time{},
			curr: time.Time{},
			next: refFirstStart,
		}, {
			name: "start of single instance",
			from: refFirstStart,
			last: time.Time{},
			curr: refFirstStart,
			next: time.Time{},
		}, {
			name: "end of single instance",
			from: refFirstEnd,
			last: refFirstStart,
			curr: time.Time{},
			next: time.Time{},
		}},
	}, {
		name: "infinitely recurring event",
		event: steven.Event{
			Start:    refStart,
			Duration: refDuration,
			Interval: refInterval,
		},
		params: []params{{
			name: "before first instance",
			from: refFirstStart.Add(-1),
			last: time.Time{},
			curr: time.Time{},
			next: refFirstStart,
		}, {
			name: "start of first instance",
			from: refFirstStart,
			last: time.Time{},
			curr: refFirstStart,
			next: refSecondStart,
		}, {
			name: "end of first instance",
			from: refFirstEnd,
			last: refFirstStart,
			curr: time.Time{},
			next: refSecondStart,
		}, {
			name: "start of second instance",
			from: refSecondStart,
			last: refFirstStart,
			curr: refSecondStart,
			next: refThirdStart,
		}, {
			name: "end of second instance",
			from: refSecondEnd,
			last: refSecondStart,
			curr: time.Time{},
			next: refThirdStart,
		}},
	}, {
		name: "recurring event with limit date",
		event: steven.Event{
			Start:      refStart,
			Duration:   refDuration,
			Interval:   refInterval,
			LimitUntil: refSecondStart.Add(1),
		},
		params: []params{{
			name: "before first instance",
			from: refFirstStart.Add(-1),
			last: time.Time{},
			curr: time.Time{},
			next: refFirstStart,
		}, {
			name: "start of first instance",
			from: refFirstStart,
			last: time.Time{},
			curr: refFirstStart,
			next: refSecondStart,
		}, {
			name: "end of first instance",
			from: refFirstEnd,
			last: refFirstStart,
			curr: time.Time{},
			next: refSecondStart,
		}, {
			name: "start of second instance",
			from: refSecondStart,
			last: refFirstStart,
			curr: refSecondStart,
			next: time.Time{},
		}, {
			name: "end of second instance",
			from: refSecondEnd,
			last: refSecondStart,
			curr: time.Time{},
			next: time.Time{},
		}},
	}, {
		name: "recurring event with limit count",
		event: steven.Event{
			Start:      refStart,
			Duration:   refDuration,
			Interval:   refInterval,
			LimitCount: 2,
		},
		params: []params{{
			name: "before first instance",
			from: refFirstStart.Add(-1),
			last: time.Time{},
			curr: time.Time{},
			next: refFirstStart,
		}, {
			name: "start of first instance",
			from: refFirstStart,
			last: time.Time{},
			curr: refFirstStart,
			next: refSecondStart,
		}, {
			name: "end of first instance",
			from: refFirstEnd,
			last: refFirstStart,
			curr: time.Time{},
			next: refSecondStart,
		}, {
			name: "start of second instance",
			from: refSecondStart,
			last: refFirstStart,
			curr: refSecondStart,
			next: time.Time{},
		}, {
			name: "end of second instance",
			from: refSecondEnd,
			last: refSecondStart,
			curr: time.Time{},
			next: time.Time{},
		}},
	}}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.event.Validate(); err != nil {
				t.Fatal(err)
			}
			for _, params := range tt.params {
				params := params
				t.Run(params.name, func(t *testing.T) {
					gotPrev := tt.event.Previous(params.from)
					gotCurr := tt.event.Current(params.from)
					gotNext := tt.event.Next(params.from)
					if wantPrev := params.last; gotPrev != wantPrev {
						t.Errorf("wrong result for previous instance\ngot:  %v\nwant: %v", gotPrev, wantPrev)
					}
					if wantCurr := params.curr; gotCurr != wantCurr {
						t.Errorf("wrong result for current instance\ngot:  %v\nwant: %v", gotCurr, wantCurr)
					}
					if wantNext := params.next; gotNext != wantNext {
						t.Errorf("wrong result for next instance\ngot:  %v\nwant: %v", gotNext, wantNext)
					}
				})
			}
		})
	}
}

func BenchmarkEventPrevious(b *testing.B) {
	benchmarkEventInstance(b, (*steven.Event).Previous)
}

func BenchmarkEventCurrent(b *testing.B) {
	benchmarkEventInstance(b, (*steven.Event).Current)
}

func BenchmarkEventNext(b *testing.B) {
	benchmarkEventInstance(b, (*steven.Event).Next)
}

func benchmarkEventInstance(b *testing.B, f func(*steven.Event, time.Time) time.Time) {
	b.Helper()
	event := steven.Event{
		Start:      refStart,
		Duration:   refDuration,
		Interval:   refInterval,
		LimitCount: 2,
	}
	if err := event.Validate(); err != nil {
		b.Fatal(err)
	}
	tests := []struct {
		name string
		from time.Time
	}{{
		name: "before first instance",
		from: event.Start.Add(-1),
	}, {
		name: "start of first instance",
		from: event.Start,
	}, {
		name: "end of first instance",
		from: event.Start.Add(event.Duration),
	}, {
		name: "event of second instance",
		from: event.Start.Add(event.Interval),
	}, {
		name: "end of second instance",
		from: event.Start.Add(event.Interval + event.Duration),
	}}
	for _, tt := range tests {
		tt := tt
		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				f(&event, tt.from)
			}
		})
	}
}
