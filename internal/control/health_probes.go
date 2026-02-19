package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type HealthProbeTarget struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Service   string    `json:"service,omitempty"`
	Endpoint  string    `json:"endpoint,omitempty"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

type HealthProbeTargetInput struct {
	Name     string `json:"name"`
	Service  string `json:"service,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Enabled  bool   `json:"enabled"`
}

type HealthProbeCheck struct {
	ID         string    `json:"id"`
	TargetID   string    `json:"target_id"`
	Status     string    `json:"status"` // healthy|degraded|unhealthy
	LatencyMS  int       `json:"latency_ms,omitempty"`
	Message    string    `json:"message,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
}

type HealthProbeCheckInput struct {
	TargetID   string    `json:"target_id"`
	Status     string    `json:"status"`
	LatencyMS  int       `json:"latency_ms,omitempty"`
	Message    string    `json:"message,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
}

type HealthProbeGateRequest struct {
	TargetIDs         []string `json:"target_ids,omitempty"`
	MinHealthyPercent int      `json:"min_healthy_percent,omitempty"`
	RecommendRollback bool     `json:"recommend_rollback,omitempty"`
}

type HealthProbeGateResult struct {
	Decision          string    `json:"decision"` // allow|block
	Reason            string    `json:"reason"`
	CheckedTargets    int       `json:"checked_targets"`
	HealthyTargets    int       `json:"healthy_targets"`
	HealthyPercent    int       `json:"healthy_percent"`
	RecommendedAction string    `json:"recommended_action,omitempty"` // continue|hold|rollback
	GeneratedAt       time.Time `json:"generated_at"`
}

type HealthProbeStore struct {
	mu           sync.RWMutex
	nextTarget   int64
	nextCheck    int64
	targets      map[string]HealthProbeTarget
	checks       map[string]HealthProbeCheck
	lastByTarget map[string]HealthProbeCheck
}

func NewHealthProbeStore() *HealthProbeStore {
	return &HealthProbeStore{
		targets:      map[string]HealthProbeTarget{},
		checks:       map[string]HealthProbeCheck{},
		lastByTarget: map[string]HealthProbeCheck{},
	}
}

func (s *HealthProbeStore) UpsertTarget(in HealthProbeTargetInput) (HealthProbeTarget, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return HealthProbeTarget{}, errors.New("name is required")
	}
	svc := strings.TrimSpace(in.Service)
	endpoint := strings.TrimSpace(in.Endpoint)
	enabled := in.Enabled
	if !in.Enabled {
		enabled = false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.targets {
		if strings.EqualFold(existing.Name, name) {
			existing.Service = svc
			existing.Endpoint = endpoint
			existing.Enabled = enabled
			existing.UpdatedAt = time.Now().UTC()
			s.targets[existing.ID] = existing
			return existing, nil
		}
	}
	s.nextTarget++
	item := HealthProbeTarget{
		ID:        "probe-" + itoa(s.nextTarget),
		Name:      name,
		Service:   svc,
		Endpoint:  endpoint,
		Enabled:   enabled,
		UpdatedAt: time.Now().UTC(),
	}
	s.targets[item.ID] = item
	return item, nil
}

func (s *HealthProbeStore) ListTargets() []HealthProbeTarget {
	s.mu.RLock()
	out := make([]HealthProbeTarget, 0, len(s.targets))
	for _, item := range s.targets {
		out = append(out, item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (s *HealthProbeStore) RecordCheck(in HealthProbeCheckInput) (HealthProbeCheck, error) {
	targetID := strings.TrimSpace(in.TargetID)
	status := normalizeProbeStatus(in.Status)
	if targetID == "" {
		return HealthProbeCheck{}, errors.New("target_id is required")
	}
	if status == "" {
		return HealthProbeCheck{}, errors.New("status must be one of healthy, degraded, unhealthy")
	}
	if in.LatencyMS < 0 {
		return HealthProbeCheck{}, errors.New("latency_ms must be >= 0")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.targets[targetID]; !ok {
		return HealthProbeCheck{}, errors.New("target not found")
	}
	observedAt := in.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	s.nextCheck++
	item := HealthProbeCheck{
		ID:         "probe-check-" + itoa(s.nextCheck),
		TargetID:   targetID,
		Status:     status,
		LatencyMS:  in.LatencyMS,
		Message:    strings.TrimSpace(in.Message),
		ObservedAt: observedAt,
	}
	s.checks[item.ID] = item
	s.lastByTarget[targetID] = item
	return item, nil
}

func (s *HealthProbeStore) EvaluateGate(req HealthProbeGateRequest) HealthProbeGateResult {
	minHealthy := req.MinHealthyPercent
	if minHealthy <= 0 {
		minHealthy = 100
	}
	if minHealthy > 100 {
		minHealthy = 100
	}
	targetFilter := map[string]struct{}{}
	for _, id := range req.TargetIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		targetFilter[id] = struct{}{}
	}

	s.mu.RLock()
	checked := 0
	healthy := 0
	for id, target := range s.targets {
		if len(targetFilter) > 0 {
			if _, ok := targetFilter[id]; !ok {
				continue
			}
		}
		if !target.Enabled {
			continue
		}
		check, ok := s.lastByTarget[id]
		if !ok {
			continue
		}
		checked++
		if check.Status == "healthy" {
			healthy++
		}
	}
	s.mu.RUnlock()

	result := HealthProbeGateResult{
		Decision:          "allow",
		Reason:            "health probes satisfy gate threshold",
		CheckedTargets:    checked,
		HealthyTargets:    healthy,
		HealthyPercent:    100,
		RecommendedAction: "continue",
		GeneratedAt:       time.Now().UTC(),
	}
	if checked == 0 {
		result.Decision = "block"
		result.Reason = "no enabled probe checks available for gate evaluation"
		result.HealthyPercent = 0
		result.RecommendedAction = "hold"
		if req.RecommendRollback {
			result.RecommendedAction = "rollback"
		}
		return result
	}
	result.HealthyPercent = (healthy * 100) / checked
	if result.HealthyPercent < minHealthy {
		result.Decision = "block"
		result.Reason = "healthy target percentage below threshold"
		result.RecommendedAction = "hold"
		if req.RecommendRollback {
			result.RecommendedAction = "rollback"
		}
	}
	return result
}

func normalizeProbeStatus(in string) string {
	switch strings.ToLower(strings.TrimSpace(in)) {
	case "healthy", "degraded", "unhealthy":
		return strings.ToLower(strings.TrimSpace(in))
	default:
		return ""
	}
}
