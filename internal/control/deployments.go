package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type DeploymentStatus string

const (
	DeploymentQueued DeploymentStatus = "queued"
)

type DeploymentTrigger struct {
	ID          string           `json:"id"`
	Environment string           `json:"environment"`
	Branch      string           `json:"branch"`
	ConfigPath  string           `json:"config_path"`
	Source      string           `json:"source"`
	Priority    string           `json:"priority,omitempty"`
	Force       bool             `json:"force,omitempty"`
	Status      DeploymentStatus `json:"status"`
	JobID       string           `json:"job_id,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
}

type DeploymentTriggerInput struct {
	Environment string `json:"environment"`
	Branch      string `json:"branch"`
	ConfigPath  string `json:"config_path"`
	Source      string `json:"source"`
	Priority    string `json:"priority,omitempty"`
	Force       bool   `json:"force,omitempty"`
	JobID       string `json:"job_id,omitempty"`
}

type DeploymentStore struct {
	mu      sync.RWMutex
	nextID  int64
	records map[string]*DeploymentTrigger
}

func NewDeploymentStore() *DeploymentStore {
	return &DeploymentStore{
		records: map[string]*DeploymentTrigger{},
	}
}

func (s *DeploymentStore) Create(in DeploymentTriggerInput) (DeploymentTrigger, error) {
	env := strings.ToLower(strings.TrimSpace(in.Environment))
	branch := strings.TrimSpace(in.Branch)
	configPath := strings.TrimSpace(in.ConfigPath)
	source := strings.ToLower(strings.TrimSpace(in.Source))
	if env == "" || branch == "" || configPath == "" {
		return DeploymentTrigger{}, errors.New("environment, branch, and config_path are required")
	}
	switch source {
	case "api", "webhook", "cli":
	default:
		return DeploymentTrigger{}, errors.New("source must be one of api, webhook, cli")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item := &DeploymentTrigger{
		ID:          "deploy-" + itoa(s.nextID),
		Environment: env,
		Branch:      branch,
		ConfigPath:  configPath,
		Source:      source,
		Priority:    normalizePriority(in.Priority),
		Force:       in.Force,
		Status:      DeploymentQueued,
		JobID:       strings.TrimSpace(in.JobID),
		CreatedAt:   now,
	}
	s.records[item.ID] = item
	return cloneDeployment(*item), nil
}

func (s *DeploymentStore) Get(id string) (DeploymentTrigger, bool) {
	id = strings.TrimSpace(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.records[id]
	if !ok {
		return DeploymentTrigger{}, false
	}
	return cloneDeployment(*item), true
}

func (s *DeploymentStore) List() []DeploymentTrigger {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]DeploymentTrigger, 0, len(s.records))
	for _, item := range s.records {
		out = append(out, cloneDeployment(*item))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func cloneDeployment(in DeploymentTrigger) DeploymentTrigger {
	return in
}
