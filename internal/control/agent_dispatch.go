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

	AgentDispatchStrategyPush   = "push"
	AgentDispatchStrategyPull   = "pull"
	AgentDispatchStrategyHybrid = "hybrid"
)

type AgentDispatchRequest struct {
	ConfigPath  string `json:"config_path"`
	Environment string `json:"environment,omitempty"`
	Priority    string `json:"priority,omitempty"`
	Force       bool   `json:"force,omitempty"`
}

type AgentEnvironmentDispatchMode struct {
	Environment string    `json:"environment"`
	Strategy    string    `json:"strategy"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AgentDispatchRecord struct {
	ID          string    `json:"id"`
	Mode        string    `json:"mode"`
	Strategy    string    `json:"strategy"`
	Environment string    `json:"environment,omitempty"`
	ConfigPath  string    `json:"config_path"`
	Priority    string    `json:"priority,omitempty"`
	Force       bool      `json:"force,omitempty"`
	Status      string    `json:"status"`
	JobID       string    `json:"job_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type AgentDispatchStore struct {
	mu       sync.RWMutex
	mode     string
	nextID   int64
	envModes map[string]AgentEnvironmentDispatchMode
	records  []AgentDispatchRecord
}

func NewAgentDispatchStore() *AgentDispatchStore {
	return &AgentDispatchStore{
		mode:     AgentDispatchModeLocal,
		envModes: map[string]AgentEnvironmentDispatchMode{},
		records:  make([]AgentDispatchRecord, 0, 256),
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

func (s *AgentDispatchStore) SetEnvironmentStrategy(environment, strategy string) (AgentEnvironmentDispatchMode, error) {
	environment = strings.TrimSpace(environment)
	if environment == "" {
		return AgentEnvironmentDispatchMode{}, errors.New("environment is required")
	}
	strategy, err := normalizeDispatchStrategy(strategy)
	if err != nil {
		return AgentEnvironmentDispatchMode{}, err
	}
	item := AgentEnvironmentDispatchMode{
		Environment: environment,
		Strategy:    strategy,
		UpdatedAt:   time.Now().UTC(),
	}
	s.mu.Lock()
	s.envModes[environment] = item
	s.mu.Unlock()
	return item, nil
}

func (s *AgentDispatchStore) GetEnvironmentStrategy(environment string) (AgentEnvironmentDispatchMode, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.envModes[strings.TrimSpace(environment)]
	if !ok {
		return AgentEnvironmentDispatchMode{}, false
	}
	return item, true
}

func (s *AgentDispatchStore) EffectiveStrategy(environment string) AgentEnvironmentDispatchMode {
	environment = strings.TrimSpace(environment)
	if environment == "" {
		environment = "default"
	}
	s.mu.RLock()
	item, ok := s.envModes[environment]
	s.mu.RUnlock()
	if ok {
		return item
	}
	return AgentEnvironmentDispatchMode{
		Environment: environment,
		Strategy:    AgentDispatchStrategyHybrid,
	}
}

func (s *AgentDispatchStore) ListEnvironmentStrategies() []AgentEnvironmentDispatchMode {
	s.mu.RLock()
	out := make([]AgentEnvironmentDispatchMode, 0, len(s.envModes))
	for _, item := range s.envModes {
		out = append(out, item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Environment < out[j].Environment })
	return out
}

func (s *AgentDispatchStore) Record(mode, strategy string, req AgentDispatchRequest, status, jobID string) AgentDispatchRecord {
	strategy, err := normalizeDispatchStrategy(strategy)
	if err != nil {
		strategy = AgentDispatchStrategyHybrid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item := AgentDispatchRecord{
		ID:          "dispatch-" + itoa(s.nextID),
		Mode:        strings.ToLower(strings.TrimSpace(mode)),
		Strategy:    strategy,
		Environment: strings.TrimSpace(req.Environment),
		ConfigPath:  strings.TrimSpace(req.ConfigPath),
		Priority:    normalizePriority(req.Priority),
		Force:       req.Force,
		Status:      strings.TrimSpace(status),
		JobID:       strings.TrimSpace(jobID),
		CreatedAt:   time.Now().UTC(),
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

func normalizeDispatchStrategy(strategy string) (string, error) {
	strategy = strings.ToLower(strings.TrimSpace(strategy))
	switch strategy {
	case AgentDispatchStrategyPush, AgentDispatchStrategyPull, AgentDispatchStrategyHybrid:
		return strategy, nil
	default:
		return "", errors.New("strategy must be push, pull, or hybrid")
	}
}
