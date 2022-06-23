package steven

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

type Server interface {
	http.Handler
}

type server struct {
	bus Bus

	newClients     chan chan<- []byte
	closingClients chan chan<- []byte
	events         chan []byte
}

func NewServer(bus Bus) (*server, error) {
	if bus == nil {
		return nil, errors.New("nil Bus")
	}
	return &server{
		bus:            bus,
		newClients:     make(chan chan<- []byte),
		closingClients: make(chan chan<- []byte),
		events:         make(chan []byte),
	}, nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO(fmrsn): Route requests.
	s.handleGetEvents(w, r)
}

func (s *server) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming support required", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ctx := r.Context()

	c := make(chan BusSync)
	unregister := s.bus.Register(ctx, c)
	defer unregister()

	ping := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-ctx.Done():
			ping.Stop()
			return

		case <-ping.C:
			fmt.Fprint(w, ":\n\n")
			flusher.Flush()

		case bsync := <-c:
			fmt.Fprint(w, "event: sync\ndata: ")
			enc := json.NewEncoder(w)
			if err := enc.Encode(bsync); err != nil {
				log.Println(err)
			}
			fmt.Fprint(w, "\n\n")
			flusher.Flush()
		}
	}
}
