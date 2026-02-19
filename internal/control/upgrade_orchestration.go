package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type UpgradeOrchestrationPlanInput struct {
	Component      string `json:"component"` // agent|control-plane
	FromChannel    string `json:"from_channel"`
	ToChannel      string `json:"to_channel"`
	Strategy       string `json:"strategy,omitempty"` // wave|canary
	TotalNodes     int    `json:"total_nodes"`
	WaveSize       int    `json:"wave_size,omitempty"`
	MaxUnavailable int    `json:"max_unavailable,omitempty"`
}

type UpgradeOrchestrationPlan struct {
	ID             string    `json:"id"`
	Component      string    `json:"component"`
	FromChannel    string    `json:"from_channel"`
	ToChannel      string    `json:"to_channel"`
	Strategy       string    `json:"strategy"`
	TotalNodes     int       `json:"total_nodes"`
	WaveSize       int       `json:"wave_size"`
	MaxUnavailable int       `json:"max_unavailable"`
	CurrentWave    int       `json:"current_wave"`
	UpgradedNodes  int       `json:"upgraded_nodes"`
	Status         string    `json:"status"` // pending|in_progress|blocked|completed|aborted
	LastMessage    string    `json:"last_message,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type UpgradeOrchestrationAdvanceInput struct {
	Healthy bool `json:"healthy"`
}

type UpgradeOrchestrationAbortInput struct {
	Reason string `json:"reason,omitempty"`
}

type UpgradeOrchestrationStore struct {
	mu     sync.RWMutex
	nextID int64
	plans  map[string]*UpgradeOrchestrationPlan
}

func NewUpgradeOrchestrationStore() *UpgradeOrchestrationStore {
	return &UpgradeOrchestrationStore{plans: map[string]*UpgradeOrchestrationPlan{}}
}

func (s *UpgradeOrchestrationStore) CreatePlan(in UpgradeOrchestrationPlanInput) (UpgradeOrchestrationPlan, error) {
	component := strings.ToLower(strings.TrimSpace(in.Component))
	fromChannel := strings.ToLower(strings.TrimSpace(in.FromChannel))
	toChannel := strings.ToLower(strings.TrimSpace(in.ToChannel))
	if component == "" || fromChannel == "" || toChannel == "" {
		return UpgradeOrchestrationPlan{}, errors.New("component, from_channel, and to_channel are required")
	}
	if component != "agent" && component != "control-plane" {
		return UpgradeOrchestrationPlan{}, errors.New("component must be agent or control-plane")
	}
	if in.TotalNodes <= 0 {
		return UpgradeOrchestrationPlan{}, errors.New("total_nodes must be greater than zero")
	}
	strategy := strings.ToLower(strings.TrimSpace(in.Strategy))
	if strategy == "" {
		strategy = "wave"
	}
	if strategy != "wave" && strategy != "canary" {
		return UpgradeOrchestrationPlan{}, errors.New("strategy must be wave or canary")
	}
	maxUnavailable := in.MaxUnavailable
	if maxUnavailable <= 0 {
		maxUnavailable = 1
	}
	waveSize := in.WaveSize
	if waveSize <= 0 {
		waveSize = maxUnavailable
	}
	if waveSize > in.TotalNodes {
		waveSize = in.TotalNodes
	}
	item := UpgradeOrchestrationPlan{
		Component:      component,
		FromChannel:    fromChannel,
		ToChannel:      toChannel,
		Strategy:       strategy,
		TotalNodes:     in.TotalNodes,
		WaveSize:       waveSize,
		MaxUnavailable: maxUnavailable,
		Status:         "pending",
		UpdatedAt:      time.Now().UTC(),
	}

	s.mu.Lock()
	s.nextID++
	item.ID = "upgrade-plan-" + itoa(s.nextID)
	s.plans[item.ID] = &item
	s.mu.Unlock()
	return item, nil
}

func (s *UpgradeOrchestrationStore) ListPlans() []UpgradeOrchestrationPlan {
	s.mu.RLock()
	out := make([]UpgradeOrchestrationPlan, 0, len(s.plans))
	for _, item := range s.plans {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (s *UpgradeOrchestrationStore) GetPlan(id string) (UpgradeOrchestrationPlan, bool) {
	s.mu.RLock()
	item, ok := s.plans[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return UpgradeOrchestrationPlan{}, false
	}
	return *item, true
}

func (s *UpgradeOrchestrationStore) Advance(id string, in UpgradeOrchestrationAdvanceInput) (UpgradeOrchestrationPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.plans[strings.TrimSpace(id)]
	if !ok {
		return UpgradeOrchestrationPlan{}, errors.New("upgrade plan not found")
	}
	if item.Status == "completed" || item.Status == "aborted" {
		return UpgradeOrchestrationPlan{}, errors.New("upgrade plan is no longer active")
	}
	if !in.Healthy {
		item.Status = "blocked"
		item.LastMessage = "health check failed; rollout blocked for zero-downtime safety"
		item.UpdatedAt = time.Now().UTC()
		return *item, nil
	}
	item.Status = "in_progress"
	item.CurrentWave++
	remaining := item.TotalNodes - item.UpgradedNodes
	step := item.WaveSize
	if step > remaining {
		step = remaining
	}
	item.UpgradedNodes += step
	if item.UpgradedNodes >= item.TotalNodes {
		item.Status = "completed"
		item.LastMessage = "upgrade completed with zero-downtime wave progression"
	} else {
		item.LastMessage = "wave advanced successfully"
	}
	item.UpdatedAt = time.Now().UTC()
	return *item, nil
}

func (s *UpgradeOrchestrationStore) Abort(id string, in UpgradeOrchestrationAbortInput) (UpgradeOrchestrationPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.plans[strings.TrimSpace(id)]
	if !ok {
		return UpgradeOrchestrationPlan{}, errors.New("upgrade plan not found")
	}
	if item.Status == "completed" {
		return UpgradeOrchestrationPlan{}, errors.New("cannot abort completed upgrade plan")
	}
	item.Status = "aborted"
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		reason = "aborted by operator"
	}
	item.LastMessage = reason
	item.UpdatedAt = time.Now().UTC()
	return *item, nil
}
