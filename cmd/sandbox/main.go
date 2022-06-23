package main

import (
	"context"
	"net/http"
	"time"

	"bsid.es/steven"
)

func now() time.Time {
	return time.Now().In(time.UTC)
}

func main() {
	e := steven.Event{
		ID:       "event",
		Name:     "event",
		Start:    now().Add(3 * time.Second).Truncate(time.Second),
		Duration: 1 * time.Second,
		Every:    2 * time.Second,
		Count:    10,
	}
	if err := e.Validate(); err != nil {
		panic(err)
	}

	sched, _ := steven.NewSched()
	sched.Now = now

	bus, _ := steven.NewBus(sched)
	bus.Now = now

	ctx := context.Background()

	sched.Run(ctx)
	bus.Run(ctx)

	sched.Reload(&e)
	bus.Reload(&e)

	broker, _ := steven.NewServer(bus)

	http.ListenAndServe(":3002", broker)
}
