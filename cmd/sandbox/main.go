package main

import (
	"context"
	"fmt"
	"time"

	"bsid.es/steven"
)

func now() time.Time {
	return time.Now().In(time.UTC)
}

func main() {
	e := steven.Event{
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

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	sched.Run(ctx)
	bus.Run(ctx)

	sched.Reload(&e)
	bus.Reload(&e)

	mainc := make(chan steven.BusEvent)
	bus.Subscribe(ctx, mainc)

	for se := range mainc {
		var typ string
		switch se.Type {
		case steven.BusEventReset:
			typ = "RESET"
		case steven.BusEventStart:
			typ = "START"
		case steven.BusEventStop:
			typ = "STOP "
		}
		for _, e := range se.Entries {
			fmt.Println(typ, "|", e.Event.Name, "|", e.Previous, "|", e.Current, "|", e.Next)
		}
	}

}
