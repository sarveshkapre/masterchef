package control

import (
	"strings"
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

type EventQuery struct {
	Since      time.Time
	Until      time.Time
	TypePrefix string
	Contains   string
	Limit      int
	Desc       bool
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

func (s *EventStore) Replace(items []Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if items == nil {
		s.events = s.events[:0]
		return
	}
	if len(items) > s.limit {
		items = items[len(items)-s.limit:]
	}
	out := make([]Event, len(items))
	copy(out, items)
	s.events = out
}

func (s *EventStore) Query(q EventQuery) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	typePrefix := strings.ToLower(strings.TrimSpace(q.TypePrefix))
	contains := strings.ToLower(strings.TrimSpace(q.Contains))
	limit := q.Limit
	if limit <= 0 {
		limit = 200
	}

	out := make([]Event, 0, minInt(limit, len(s.events)))
	appendIfMatch := func(e Event) bool {
		if !q.Since.IsZero() && e.Time.Before(q.Since) {
			return false
		}
		if !q.Until.IsZero() && e.Time.After(q.Until) {
			return false
		}
		if typePrefix != "" && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(e.Type)), typePrefix) {
			return false
		}
		if contains != "" {
			msg := strings.ToLower(e.Message)
			typ := strings.ToLower(e.Type)
			if !strings.Contains(msg, contains) && !strings.Contains(typ, contains) {
				return false
			}
		}
		out = append(out, e)
		return len(out) >= limit
	}
	if q.Desc {
		for i := len(s.events) - 1; i >= 0; i-- {
			if appendIfMatch(s.events[i]) {
				break
			}
		}
		return out
	}
	for i := 0; i < len(s.events); i++ {
		if appendIfMatch(s.events[i]) {
			break
		}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
