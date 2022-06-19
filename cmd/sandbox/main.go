package main

import (
	"context"
	"fmt"
	"time"

	"bsid.es/steven"
)

func main() {
	e := steven.Event{
		Name:     "event",
		Start:    time.Now().Add(3 * time.Second),
		Duration: 1 * time.Second,
		Every:    1 * time.Second,
		Count:    2,
	}
	if err := e.Validate(); err != nil {
		panic(err)
	}

	sched := steven.NewScheduler()

	sched.Reload(&e)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sched.Run(ctx)

	for se := range sched.Events() {
		switch se.Type {
		case steven.SchedStart:
			fmt.Println("START |", se.Event.Name, "|", se.Instance)
		case steven.SchedStop:
			fmt.Println("STOP  |", se.Event.Name, "|", se.Instance)
		}
	}

}
