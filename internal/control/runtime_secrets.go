package control

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type RuntimeSecretSession struct {
	ID          string     `json:"id"`
	Source      string     `json:"source"`
	TTLSeconds  int        `json:"ttl_seconds"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	Consumed    bool       `json:"consumed"`
	ConsumedAt  *time.Time `json:"consumed_at,omitempty"`
	Destroyed   bool       `json:"destroyed"`
	DestroyedAt *time.Time `json:"destroyed_at,omitempty"`
}

type RuntimeSecretSessionInput struct {
	Source     string         `json:"source"`
	Data       map[string]any `json:"data"`
	TTLSeconds int            `json:"ttl_seconds,omitempty"`
}

type runtimeSecretRecord struct {
	session RuntimeSecretSession
	payload []byte
}

type RuntimeSecretStore struct {
	mu       sync.RWMutex
	nextID   int64
	sessions map[string]*runtimeSecretRecord
}

func NewRuntimeSecretStore() *RuntimeSecretStore {
	return &RuntimeSecretStore{
		sessions: map[string]*runtimeSecretRecord{},
	}
}

func (s *RuntimeSecretStore) Materialize(in RuntimeSecretSessionInput) (RuntimeSecretSession, error) {
	source := strings.TrimSpace(in.Source)
	if source == "" {
		return RuntimeSecretSession{}, errors.New("source is required")
	}
	if in.Data == nil {
		in.Data = map[string]any{}
	}
	payload, err := json.Marshal(in.Data)
	if err != nil {
		return RuntimeSecretSession{}, err
	}
	ttl := in.TTLSeconds
	if ttl <= 0 {
		ttl = 300
	}
	if ttl < 30 {
		return RuntimeSecretSession{}, errors.New("ttl_seconds must be >= 30")
	}
	if ttl > 3600 {
		return RuntimeSecretSession{}, errors.New("ttl_seconds must be <= 3600")
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked(now)
	s.nextID++
	session := RuntimeSecretSession{
		ID:         "runtime-secret-" + itoa(s.nextID),
		Source:     source,
		TTLSeconds: ttl,
		CreatedAt:  now,
		ExpiresAt:  now.Add(time.Duration(ttl) * time.Second),
	}
	s.sessions[session.ID] = &runtimeSecretRecord{
		session: session,
		payload: append([]byte{}, payload...),
	}
	return cloneRuntimeSecretSession(session), nil
}

func (s *RuntimeSecretStore) List() []RuntimeSecretSession {
	now := time.Now().UTC()
	s.mu.Lock()
	s.cleanupExpiredLocked(now)
	out := make([]RuntimeSecretSession, 0, len(s.sessions))
	for _, item := range s.sessions {
		out = append(out, cloneRuntimeSecretSession(item.session))
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *RuntimeSecretStore) Get(id string) (RuntimeSecretSession, bool) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked(now)
	item, ok := s.sessions[strings.TrimSpace(id)]
	if !ok {
		return RuntimeSecretSession{}, false
	}
	return cloneRuntimeSecretSession(item.session), true
}

func (s *RuntimeSecretStore) Consume(id string) (map[string]any, RuntimeSecretSession, error) {
	return s.consumeAt(id, time.Now().UTC())
}

func (s *RuntimeSecretStore) consumeAt(id string, now time.Time) (map[string]any, RuntimeSecretSession, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, RuntimeSecretSession{}, errors.New("session_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked(now)
	record, ok := s.sessions[id]
	if !ok {
		return nil, RuntimeSecretSession{}, errors.New("runtime secret session not found")
	}
	if record.session.Destroyed {
		return nil, cloneRuntimeSecretSession(record.session), errors.New("runtime secret session destroyed")
	}
	if record.session.Consumed {
		return nil, cloneRuntimeSecretSession(record.session), errors.New("runtime secret session already consumed")
	}
	if !now.Before(record.session.ExpiresAt) {
		s.zeroizeRecordLocked(id, record, now, true)
		return nil, cloneRuntimeSecretSession(record.session), errors.New("runtime secret session expired")
	}
	var out map[string]any
	if err := json.Unmarshal(record.payload, &out); err != nil {
		return nil, RuntimeSecretSession{}, err
	}
	consumedAt := now
	record.session.Consumed = true
	record.session.ConsumedAt = &consumedAt
	s.zeroizePayload(record.payload)
	record.payload = nil
	return out, cloneRuntimeSecretSession(record.session), nil
}

func (s *RuntimeSecretStore) Destroy(id string) (RuntimeSecretSession, error) {
	now := time.Now().UTC()
	id = strings.TrimSpace(id)
	if id == "" {
		return RuntimeSecretSession{}, errors.New("session_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.sessions[id]
	if !ok {
		return RuntimeSecretSession{}, errors.New("runtime secret session not found")
	}
	s.zeroizeRecordLocked(id, record, now, false)
	return cloneRuntimeSecretSession(record.session), nil
}

func (s *RuntimeSecretStore) cleanupExpiredLocked(now time.Time) {
	for id, record := range s.sessions {
		if !now.Before(record.session.ExpiresAt) && !record.session.Destroyed {
			s.zeroizeRecordLocked(id, record, now, true)
		}
	}
}

func (s *RuntimeSecretStore) zeroizeRecordLocked(_ string, record *runtimeSecretRecord, now time.Time, expired bool) {
	if record.payload != nil {
		s.zeroizePayload(record.payload)
		record.payload = nil
	}
	if expired {
		record.session.Consumed = true
		if record.session.ConsumedAt == nil {
			consumedAt := now
			record.session.ConsumedAt = &consumedAt
		}
	}
	record.session.Destroyed = true
	if record.session.DestroyedAt == nil {
		destroyedAt := now
		record.session.DestroyedAt = &destroyedAt
	}
}

func (s *RuntimeSecretStore) zeroizePayload(payload []byte) {
	for i := range payload {
		payload[i] = 0
	}
}

func cloneRuntimeSecretSession(in RuntimeSecretSession) RuntimeSecretSession {
	out := in
	if in.ConsumedAt != nil {
		consumedAt := *in.ConsumedAt
		out.ConsumedAt = &consumedAt
	}
	if in.DestroyedAt != nil {
		destroyedAt := *in.DestroyedAt
		out.DestroyedAt = &destroyedAt
	}
	return out
}
