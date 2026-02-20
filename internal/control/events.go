package control

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

type Event struct {
	Index    int64          `json:"index,omitempty"`
	Time     time.Time      `json:"time"`
	Type     string         `json:"type"`
	Message  string         `json:"message"`
	Fields   map[string]any `json:"fields,omitempty"`
	PrevHash string         `json:"prev_hash,omitempty"`
	Hash     string         `json:"hash,omitempty"`
}

type EventIntegrityViolation struct {
	Index        int64  `json:"index"`
	Reason       string `json:"reason"`
	ExpectedHash string `json:"expected_hash,omitempty"`
	ActualHash   string `json:"actual_hash,omitempty"`
}

type EventIntegrityReport struct {
	Valid      bool                      `json:"valid"`
	Checked    int                       `json:"checked"`
	LastHash   string                    `json:"last_hash,omitempty"`
	Violations []EventIntegrityViolation `json:"violations,omitempty"`
}

type EventStore struct {
	mu               sync.RWMutex
	events           []Event
	limit            int
	nextIndex        int64
	lastHash         string
	nextSubscriberID int64
	subscribers      map[int64]chan Event
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
		events:      make([]Event, 0, limit),
		limit:       limit,
		subscribers: map[int64]chan Event{},
	}
}

func (s *EventStore) Append(e Event) {
	s.mu.Lock()
	sealed := s.sealEventLocked(e)
	if len(s.events) >= s.limit {
		copy(s.events[0:], s.events[1:])
		s.events[len(s.events)-1] = sealed
	} else {
		s.events = append(s.events, sealed)
	}
	subs := make([]chan Event, 0, len(s.subscribers))
	for _, ch := range s.subscribers {
		subs = append(subs, ch)
	}
	s.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- sealed:
		default:
		}
	}
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
		s.lastHash = ""
		s.nextIndex = 0
		return
	}
	s.events = s.events[:0]
	s.lastHash = ""
	s.nextIndex = 0
	if len(items) > s.limit {
		items = items[len(items)-s.limit:]
	}
	for _, item := range items {
		s.events = append(s.events, s.sealEventLocked(item))
	}
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

func (s *EventStore) VerifyIntegrity() EventIntegrityReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	report := EventIntegrityReport{
		Valid:      true,
		Checked:    len(s.events),
		Violations: make([]EventIntegrityViolation, 0),
	}
	var prevHash string
	for i, event := range s.events {
		expectedIndex := int64(i + 1)
		if event.Index != expectedIndex {
			report.Valid = false
			report.Violations = append(report.Violations, EventIntegrityViolation{
				Index:  event.Index,
				Reason: "event index sequence mismatch",
			})
		}
		if strings.TrimSpace(event.PrevHash) != strings.TrimSpace(prevHash) {
			report.Valid = false
			report.Violations = append(report.Violations, EventIntegrityViolation{
				Index:  event.Index,
				Reason: "prev_hash mismatch",
			})
		}
		expectedHash := computeEventHash(event.Index, event.Time, event.Type, event.Message, event.Fields, prevHash)
		if strings.TrimSpace(event.Hash) != expectedHash {
			report.Valid = false
			report.Violations = append(report.Violations, EventIntegrityViolation{
				Index:        event.Index,
				Reason:       "hash mismatch",
				ExpectedHash: expectedHash,
				ActualHash:   event.Hash,
			})
		}
		prevHash = expectedHash
	}
	report.LastHash = prevHash
	return report
}

func (s *EventStore) Subscribe(buffer int) (int64, <-chan Event) {
	if buffer <= 0 {
		buffer = 64
	}
	ch := make(chan Event, buffer)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextSubscriberID++
	id := s.nextSubscriberID
	s.subscribers[id] = ch
	return id, ch
}

func (s *EventStore) Unsubscribe(id int64) {
	s.mu.Lock()
	ch, ok := s.subscribers[id]
	if ok {
		delete(s.subscribers, id)
	}
	s.mu.Unlock()
	if ok {
		close(ch)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *EventStore) sealEventLocked(event Event) Event {
	sealed := event
	if sealed.Time.IsZero() {
		sealed.Time = time.Now().UTC()
	}
	s.nextIndex++
	sealed.Index = s.nextIndex
	sealed.PrevHash = s.lastHash
	sealed.Hash = computeEventHash(sealed.Index, sealed.Time, sealed.Type, sealed.Message, sealed.Fields, sealed.PrevHash)
	s.lastHash = sealed.Hash
	return sealed
}

func computeEventHash(index int64, ts time.Time, eventType, message string, fields map[string]any, prevHash string) string {
	payload := map[string]any{
		"index":     index,
		"time":      ts.UTC().Format(time.RFC3339Nano),
		"type":      strings.TrimSpace(eventType),
		"message":   strings.TrimSpace(message),
		"fields":    fields,
		"prev_hash": strings.TrimSpace(prevHash),
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
