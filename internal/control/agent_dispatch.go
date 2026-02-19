package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	AgentDispatchModeLocal    = "local"
	AgentDispatchModeEventBus = "event_bus"
)

type AgentDispatchRequest struct {
	ConfigPath string `json:"config_path"`
	Priority   string `json:"priority,omitempty"`
	Force      bool   `json:"force,omitempty"`
}

type AgentDispatchRecord struct {
	ID         string    `json:"id"`
	Mode       string    `json:"mode"`
	ConfigPath string    `json:"config_path"`
	Priority   string    `json:"priority,omitempty"`
	Force      bool      `json:"force,omitempty"`
	Status     string    `json:"status"`
	JobID      string    `json:"job_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type AgentDispatchStore struct {
	mu      sync.RWMutex
	mode    string
	nextID  int64
	records []AgentDispatchRecord
}

func NewAgentDispatchStore() *AgentDispatchStore {
	return &AgentDispatchStore{
		mode:    AgentDispatchModeLocal,
		records: make([]AgentDispatchRecord, 0, 256),
	}
}

func (s *AgentDispatchStore) Mode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

func (s *AgentDispatchStore) SetMode(mode string) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case AgentDispatchModeLocal, AgentDispatchModeEventBus:
	default:
		return "", errors.New("mode must be local or event_bus")
	}
	s.mu.Lock()
	s.mode = mode
	s.mu.Unlock()
	return mode, nil
}

func (s *AgentDispatchStore) Record(mode string, req AgentDispatchRequest, status, jobID string) AgentDispatchRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item := AgentDispatchRecord{
		ID:         "dispatch-" + itoa(s.nextID),
		Mode:       strings.ToLower(strings.TrimSpace(mode)),
		ConfigPath: strings.TrimSpace(req.ConfigPath),
		Priority:   normalizePriority(req.Priority),
		Force:      req.Force,
		Status:     strings.TrimSpace(status),
		JobID:      strings.TrimSpace(jobID),
		CreatedAt:  time.Now().UTC(),
	}
	s.records = append(s.records, item)
	if len(s.records) > 2000 {
		s.records = s.records[len(s.records)-2000:]
	}
	return item
}

func (s *AgentDispatchStore) List(limit int) []AgentDispatchRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]AgentDispatchRecord{}, s.records...)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
