package control

import (
	"sync"
	"time"
)

type Event struct {
	Time    time.Time      `json:"time"`
	Type    string         `json:"type"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

type EventStore struct {
	mu     sync.RWMutex
	events []Event
	limit  int
}

func NewEventStore(limit int) *EventStore {
	if limit <= 0 {
		limit = 10_000
	}
	return &EventStore{
		events: make([]Event, 0, limit),
		limit:  limit,
	}
}

func (s *EventStore) Append(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	if len(s.events) >= s.limit {
		copy(s.events[0:], s.events[1:])
		s.events[len(s.events)-1] = e
		return
	}
	s.events = append(s.events, e)
}

func (s *EventStore) List() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Event, len(s.events))
	copy(out, s.events)
	return out
}
