package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type PerformanceGatePolicy struct {
	MaxP95LatencyMS    int64     `json:"max_p95_latency_ms"`
	MinThroughputRPS   float64   `json:"min_throughput_rps"`
	MaxErrorBudgetBurn float64   `json:"max_error_budget_burn"`
	MinSampleCount     int       `json:"min_sample_count"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type PerformanceGateSample struct {
	Component           string  `json:"component"`
	P95LatencyMS        int64   `json:"p95_latency_ms"`
	ThroughputRPS       float64 `json:"throughput_rps"`
	ErrorBudgetBurnRate float64 `json:"error_budget_burn_rate"`
	SampleCount         int     `json:"sample_count"`
}

type PerformanceGateEvaluation struct {
	ID              string    `json:"id"`
	Component       string    `json:"component"`
	Pass            bool      `json:"pass"`
	P95LatencyMS    int64     `json:"p95_latency_ms"`
	ThroughputRPS   float64   `json:"throughput_rps"`
	ErrorBudgetBurn float64   `json:"error_budget_burn"`
	SampleCount     int       `json:"sample_count"`
	Score           float64   `json:"score"`
	BlockReasons    []string  `json:"block_reasons,omitempty"`
	EvaluatedAt     time.Time `json:"evaluated_at"`
}

type PerformanceGateStore struct {
	mu      sync.RWMutex
	nextID  int64
	policy  PerformanceGatePolicy
	history []PerformanceGateEvaluation
}

func NewPerformanceGateStore() *PerformanceGateStore {
	return &PerformanceGateStore{
		policy: PerformanceGatePolicy{
			MaxP95LatencyMS:    2000,
			MinThroughputRPS:   100,
			MaxErrorBudgetBurn: 1.0,
			MinSampleCount:     100,
			UpdatedAt:          time.Now().UTC(),
		},
		history: make([]PerformanceGateEvaluation, 0),
	}
}

func (s *PerformanceGateStore) Policy() PerformanceGatePolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy
}

func (s *PerformanceGateStore) SetPolicy(in PerformanceGatePolicy) (PerformanceGatePolicy, error) {
	if in.MaxP95LatencyMS <= 0 {
		return PerformanceGatePolicy{}, errors.New("max_p95_latency_ms must be greater than zero")
	}
	if in.MinThroughputRPS <= 0 {
		return PerformanceGatePolicy{}, errors.New("min_throughput_rps must be greater than zero")
	}
	if in.MaxErrorBudgetBurn < 0 {
		return PerformanceGatePolicy{}, errors.New("max_error_budget_burn must be non-negative")
	}
	if in.MinSampleCount <= 0 {
		in.MinSampleCount = 100
	}
	in.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	s.policy = in
	s.mu.Unlock()
	return in, nil
}

func (s *PerformanceGateStore) Evaluate(in PerformanceGateSample) (PerformanceGateEvaluation, error) {
	component := strings.TrimSpace(in.Component)
	if component == "" {
		return PerformanceGateEvaluation{}, errors.New("component is required")
	}
	if in.P95LatencyMS < 0 || in.ThroughputRPS < 0 || in.ErrorBudgetBurnRate < 0 || in.SampleCount < 0 {
		return PerformanceGateEvaluation{}, errors.New("performance sample metrics must be non-negative")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	policy := s.policy
	reasons := make([]string, 0)
	if in.SampleCount < policy.MinSampleCount {
		reasons = append(reasons, "sample_count below minimum threshold")
	}
	if in.P95LatencyMS > policy.MaxP95LatencyMS {
		reasons = append(reasons, "p95 latency exceeded threshold")
	}
	if in.ThroughputRPS < policy.MinThroughputRPS {
		reasons = append(reasons, "throughput below threshold")
	}
	if in.ErrorBudgetBurnRate > policy.MaxErrorBudgetBurn {
		reasons = append(reasons, "error budget burn exceeded threshold")
	}

	score := 100.0
	if in.P95LatencyMS > policy.MaxP95LatencyMS {
		over := float64(in.P95LatencyMS-policy.MaxP95LatencyMS) / float64(policy.MaxP95LatencyMS)
		score -= over * 40
	}
	if in.ThroughputRPS < policy.MinThroughputRPS {
		deficit := (policy.MinThroughputRPS - in.ThroughputRPS) / policy.MinThroughputRPS
		score -= deficit * 35
	}
	if in.ErrorBudgetBurnRate > policy.MaxErrorBudgetBurn {
		over := (in.ErrorBudgetBurnRate - policy.MaxErrorBudgetBurn) / maxPerformanceFloat(policy.MaxErrorBudgetBurn, 0.0001)
		score -= over * 25
	}
	if in.SampleCount < policy.MinSampleCount {
		score -= 10
	}
	if score < 0 {
		score = 0
	}

	s.nextID++
	item := PerformanceGateEvaluation{
		ID:              "perf-gate-eval-" + itoa(s.nextID),
		Component:       component,
		Pass:            len(reasons) == 0,
		P95LatencyMS:    in.P95LatencyMS,
		ThroughputRPS:   in.ThroughputRPS,
		ErrorBudgetBurn: in.ErrorBudgetBurnRate,
		SampleCount:     in.SampleCount,
		Score:           score,
		BlockReasons:    reasons,
		EvaluatedAt:     time.Now().UTC(),
	}
	s.history = append(s.history, item)
	if len(s.history) > 500 {
		s.history = s.history[len(s.history)-500:]
	}
	return clonePerformanceGateEvaluation(item), nil
}

func (s *PerformanceGateStore) List(limit int) []PerformanceGateEvaluation {
	if limit <= 0 {
		limit = 100
	}
	s.mu.RLock()
	out := make([]PerformanceGateEvaluation, len(s.history))
	copy(out, s.history)
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		return out[i].EvaluatedAt.After(out[j].EvaluatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	for i := range out {
		out[i] = clonePerformanceGateEvaluation(out[i])
	}
	return out
}

func clonePerformanceGateEvaluation(in PerformanceGateEvaluation) PerformanceGateEvaluation {
	in.BlockReasons = cloneStringSlice(in.BlockReasons)
	return in
}

func maxPerformanceFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
