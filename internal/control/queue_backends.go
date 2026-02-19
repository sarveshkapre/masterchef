package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type QueueBackendInput struct {
	Name              string `json:"name"`
	Type              string `json:"type"` // embedded|redis|nats|sqs|rabbitmq|kafka
	DSN               string `json:"dsn,omitempty"`
	MaxInFlight       int    `json:"max_in_flight,omitempty"`
	AckTimeoutSeconds int    `json:"ack_timeout_seconds,omitempty"`
	Enabled           bool   `json:"enabled"`
}

type QueueBackend struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Type              string    `json:"type"`
	DSN               string    `json:"dsn,omitempty"`
	MaxInFlight       int       `json:"max_in_flight"`
	AckTimeoutSeconds int       `json:"ack_timeout_seconds"`
	Enabled           bool      `json:"enabled"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type QueueBackendPolicy struct {
	ActiveBackendID    string    `json:"active_backend_id"`
	FailoverBackendIDs []string  `json:"failover_backend_ids,omitempty"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type QueueBackendPolicyInput struct {
	ActiveBackendID    string   `json:"active_backend_id"`
	FailoverBackendIDs []string `json:"failover_backend_ids,omitempty"`
}

type QueueBackendAdmitInput struct {
	RequireExternal bool `json:"require_external,omitempty"`
}

type QueueBackendAdmitResult struct {
	Allowed         bool     `json:"allowed"`
	SelectedBackend string   `json:"selected_backend,omitempty"`
	Failovers       []string `json:"failovers,omitempty"`
	Reason          string   `json:"reason"`
}

type QueueBackendStore struct {
	mu       sync.RWMutex
	nextID   int64
	backends map[string]*QueueBackend
	policy   QueueBackendPolicy
}

func NewQueueBackendStore() *QueueBackendStore {
	store := &QueueBackendStore{
		backends: map[string]*QueueBackend{},
	}
	embedded, _ := store.Upsert(QueueBackendInput{
		Name:              "embedded-default",
		Type:              "embedded",
		MaxInFlight:       512,
		AckTimeoutSeconds: 30,
		Enabled:           true,
	})
	store.policy = QueueBackendPolicy{
		ActiveBackendID: embedded.ID,
		UpdatedAt:       time.Now().UTC(),
	}
	return store
}

func (s *QueueBackendStore) Upsert(in QueueBackendInput) (QueueBackend, error) {
	name := strings.TrimSpace(in.Name)
	kind := strings.ToLower(strings.TrimSpace(in.Type))
	if name == "" || kind == "" {
		return QueueBackend{}, errors.New("name and type are required")
	}
	switch kind {
	case "embedded", "redis", "nats", "sqs", "rabbitmq", "kafka":
	default:
		return QueueBackend{}, errors.New("type must be embedded, redis, nats, sqs, rabbitmq, or kafka")
	}
	item := QueueBackend{
		Name:              name,
		Type:              kind,
		DSN:               strings.TrimSpace(in.DSN),
		MaxInFlight:       in.MaxInFlight,
		AckTimeoutSeconds: in.AckTimeoutSeconds,
		Enabled:           in.Enabled,
		UpdatedAt:         time.Now().UTC(),
	}
	if item.MaxInFlight <= 0 {
		item.MaxInFlight = 512
	}
	if item.AckTimeoutSeconds <= 0 {
		item.AckTimeoutSeconds = 30
	}
	if kind != "embedded" && item.DSN == "" {
		return QueueBackend{}, errors.New("dsn is required for external queue backends")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.backends {
		if strings.EqualFold(existing.Name, item.Name) {
			item.ID = existing.ID
			s.backends[item.ID] = &item
			return item, nil
		}
	}
	s.nextID++
	item.ID = "queue-backend-" + itoa(s.nextID)
	s.backends[item.ID] = &item
	return item, nil
}

func (s *QueueBackendStore) List() []QueueBackend {
	s.mu.RLock()
	out := make([]QueueBackend, 0, len(s.backends))
	for _, item := range s.backends {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *QueueBackendStore) Get(id string) (QueueBackend, bool) {
	s.mu.RLock()
	item, ok := s.backends[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return QueueBackend{}, false
	}
	return *item, true
}

func (s *QueueBackendStore) Policy() QueueBackendPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return QueueBackendPolicy{
		ActiveBackendID:    s.policy.ActiveBackendID,
		FailoverBackendIDs: append([]string{}, s.policy.FailoverBackendIDs...),
		UpdatedAt:          s.policy.UpdatedAt,
	}
}

func (s *QueueBackendStore) SetPolicy(in QueueBackendPolicyInput) (QueueBackendPolicy, error) {
	activeID := strings.TrimSpace(in.ActiveBackendID)
	if activeID == "" {
		return QueueBackendPolicy{}, errors.New("active_backend_id is required")
	}
	failovers := normalizeStringList(in.FailoverBackendIDs)

	s.mu.Lock()
	defer s.mu.Unlock()
	active, ok := s.backends[activeID]
	if !ok {
		return QueueBackendPolicy{}, errors.New("active backend not found")
	}
	if !active.Enabled {
		return QueueBackendPolicy{}, errors.New("active backend is disabled")
	}
	validatedFailovers := make([]string, 0, len(failovers))
	for _, id := range failovers {
		if id == activeID {
			continue
		}
		backend, ok := s.backends[id]
		if !ok {
			return QueueBackendPolicy{}, errors.New("failover backend not found: " + id)
		}
		if !backend.Enabled {
			continue
		}
		validatedFailovers = append(validatedFailovers, id)
	}
	s.policy = QueueBackendPolicy{
		ActiveBackendID:    activeID,
		FailoverBackendIDs: validatedFailovers,
		UpdatedAt:          time.Now().UTC(),
	}
	return s.policy, nil
}

func (s *QueueBackendStore) Admit(in QueueBackendAdmitInput) QueueBackendAdmitResult {
	s.mu.RLock()
	policy := s.policy
	active, ok := s.backends[policy.ActiveBackendID]
	failovers := make([]string, 0, len(policy.FailoverBackendIDs))
	for _, id := range policy.FailoverBackendIDs {
		if backend, ok := s.backends[id]; ok && backend.Enabled {
			failovers = append(failovers, id)
		}
	}
	s.mu.RUnlock()
	if !ok || !active.Enabled {
		return QueueBackendAdmitResult{
			Allowed: false,
			Reason:  "active queue backend is unavailable",
		}
	}
	if in.RequireExternal && active.Type == "embedded" {
		return QueueBackendAdmitResult{
			Allowed:   false,
			Reason:    "external queue backend is required but active backend is embedded",
			Failovers: failovers,
		}
	}
	return QueueBackendAdmitResult{
		Allowed:         true,
		SelectedBackend: active.ID,
		Failovers:       failovers,
		Reason:          "queue backend policy is admissible",
	}
}
