package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ConvergeTriggerStatus string

const (
	ConvergeTriggerRecorded ConvergeTriggerStatus = "recorded"
	ConvergeTriggerQueued   ConvergeTriggerStatus = "queued"
	ConvergeTriggerBlocked  ConvergeTriggerStatus = "blocked"
)

type ConvergeTrigger struct {
	ID             string                `json:"id"`
	Source         string                `json:"source"` // policy|package|security|manual
	EventType      string                `json:"event_type,omitempty"`
	EventID        string                `json:"event_id,omitempty"`
	ConfigPath     string                `json:"config_path"`
	Priority       string                `json:"priority"`
	IdempotencyKey string                `json:"idempotency_key,omitempty"`
	Force          bool                  `json:"force,omitempty"`
	AutoEnqueue    bool                  `json:"auto_enqueue"`
	Status         ConvergeTriggerStatus `json:"status"`
	JobID          string                `json:"job_id,omitempty"`
	EnqueueError   string                `json:"enqueue_error,omitempty"`
	Payload        map[string]any        `json:"payload,omitempty"`
	CreatedAt      time.Time             `json:"created_at"`
}

type ConvergeTriggerInput struct {
	Source         string         `json:"source"`
	EventType      string         `json:"event_type,omitempty"`
	EventID        string         `json:"event_id,omitempty"`
	ConfigPath     string         `json:"config_path"`
	Priority       string         `json:"priority,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Force          bool           `json:"force,omitempty"`
	AutoEnqueue    bool           `json:"auto_enqueue,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
}

type ConvergeTriggerStore struct {
	mu      sync.RWMutex
	nextID  int64
	max     int
	items   map[string]ConvergeTrigger
	ordered []string
}

func NewConvergeTriggerStore(max int) *ConvergeTriggerStore {
	if max <= 0 {
		max = 2000
	}
	return &ConvergeTriggerStore{
		max:   max,
		items: map[string]ConvergeTrigger{},
	}
}

func (s *ConvergeTriggerStore) NewTrigger(in ConvergeTriggerInput) (ConvergeTrigger, error) {
	source := normalizeTriggerSource(in.Source)
	if source == "" {
		return ConvergeTrigger{}, errors.New("source must be one of policy, package, security, manual")
	}
	configPath := strings.TrimSpace(in.ConfigPath)
	if configPath == "" {
		return ConvergeTrigger{}, errors.New("config_path is required")
	}
	trigger := ConvergeTrigger{
		Source:         source,
		EventType:      strings.TrimSpace(in.EventType),
		EventID:        strings.TrimSpace(in.EventID),
		ConfigPath:     configPath,
		Priority:       normalizePriority(in.Priority),
		IdempotencyKey: strings.TrimSpace(in.IdempotencyKey),
		Force:          in.Force,
		AutoEnqueue:    in.AutoEnqueue,
		Status:         ConvergeTriggerRecorded,
		Payload:        cloneConvergePayload(in.Payload),
		CreatedAt:      time.Now().UTC(),
	}
	if !in.AutoEnqueue {
		trigger.AutoEnqueue = false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	trigger.ID = "trg-" + itoa(s.nextID)
	s.items[trigger.ID] = cloneConvergeTrigger(trigger)
	s.ordered = append(s.ordered, trigger.ID)
	s.trimLocked()
	return trigger, nil
}

func (s *ConvergeTriggerStore) UpdateOutcome(id string, status ConvergeTriggerStatus, jobID, enqueueErr string) (ConvergeTrigger, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[strings.TrimSpace(id)]
	if !ok {
		return ConvergeTrigger{}, false
	}
	item.Status = status
	item.JobID = strings.TrimSpace(jobID)
	item.EnqueueError = strings.TrimSpace(enqueueErr)
	s.items[item.ID] = cloneConvergeTrigger(item)
	return cloneConvergeTrigger(item), true
}

func (s *ConvergeTriggerStore) Get(id string) (ConvergeTrigger, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[strings.TrimSpace(id)]
	if !ok {
		return ConvergeTrigger{}, false
	}
	return cloneConvergeTrigger(item), true
}

func (s *ConvergeTriggerStore) List(limit int) []ConvergeTrigger {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.ordered) {
		limit = len(s.ordered)
	}
	out := make([]ConvergeTrigger, 0, limit)
	for i := len(s.ordered) - 1; i >= 0 && len(out) < limit; i-- {
		id := s.ordered[i]
		out = append(out, cloneConvergeTrigger(s.items[id]))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (s *ConvergeTriggerStore) trimLocked() {
	if s.max <= 0 || len(s.ordered) <= s.max {
		return
	}
	drop := len(s.ordered) - s.max
	for i := 0; i < drop; i++ {
		id := s.ordered[0]
		s.ordered = s.ordered[1:]
		delete(s.items, id)
	}
}

func normalizeTriggerSource(in string) string {
	switch strings.ToLower(strings.TrimSpace(in)) {
	case "policy", "package", "security", "manual":
		return strings.ToLower(strings.TrimSpace(in))
	default:
		return ""
	}
}

func cloneConvergeTrigger(in ConvergeTrigger) ConvergeTrigger {
	out := in
	out.Payload = cloneConvergePayload(in.Payload)
	return out
}

func cloneConvergePayload(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[strings.TrimSpace(k)] = v
	}
	return out
}
