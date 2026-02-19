package control

import (
	"strings"
	"sync"
	"time"
)

type AdaptiveConcurrencyPolicy struct {
	MinParallelism              int       `json:"min_parallelism"`
	MaxParallelism              int       `json:"max_parallelism"`
	DefaultParallelism          int       `json:"default_parallelism"`
	FailureRateScaleDownStart   float64   `json:"failure_rate_scale_down_start"`
	FailureRateCritical         float64   `json:"failure_rate_critical"`
	DegradedHostPenaltyPercent  int       `json:"degraded_host_penalty_percent"`
	UnhealthyHostPenaltyPercent int       `json:"unhealthy_host_penalty_percent"`
	UpdatedAt                   time.Time `json:"updated_at"`
}

type AdaptiveConcurrencyInput struct {
	CurrentParallelism int               `json:"current_parallelism,omitempty"`
	RecentFailureRate  float64           `json:"recent_failure_rate,omitempty"`
	HostHealth         map[string]string `json:"host_health,omitempty"`
	Backlog            int               `json:"backlog,omitempty"`
}

type AdaptiveConcurrencyDecision struct {
	RecommendedParallelism int      `json:"recommended_parallelism"`
	Reasons                []string `json:"reasons"`
}

type AdaptiveConcurrencyStore struct {
	mu     sync.RWMutex
	policy AdaptiveConcurrencyPolicy
}

func NewAdaptiveConcurrencyStore() *AdaptiveConcurrencyStore {
	return &AdaptiveConcurrencyStore{
		policy: AdaptiveConcurrencyPolicy{
			MinParallelism:              1,
			MaxParallelism:              50,
			DefaultParallelism:          10,
			FailureRateScaleDownStart:   0.10,
			FailureRateCritical:         0.30,
			DegradedHostPenaltyPercent:  15,
			UnhealthyHostPenaltyPercent: 35,
			UpdatedAt:                   time.Now().UTC(),
		},
	}
}

func (s *AdaptiveConcurrencyStore) Policy() AdaptiveConcurrencyPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy
}

func (s *AdaptiveConcurrencyStore) SetPolicy(in AdaptiveConcurrencyPolicy) AdaptiveConcurrencyPolicy {
	normalized := normalizeAdaptivePolicy(in)
	s.mu.Lock()
	s.policy = normalized
	s.mu.Unlock()
	return normalized
}

func (s *AdaptiveConcurrencyStore) Recommend(in AdaptiveConcurrencyInput) AdaptiveConcurrencyDecision {
	policy := s.Policy()
	base := in.CurrentParallelism
	if base <= 0 {
		base = policy.DefaultParallelism
	}
	recommended := clampInt(base, policy.MinParallelism, policy.MaxParallelism)
	reasons := make([]string, 0, 5)

	if in.RecentFailureRate >= policy.FailureRateCritical {
		recommended = policy.MinParallelism
		reasons = append(reasons, "critical failure rate reached; forcing minimum parallelism")
	} else if in.RecentFailureRate >= policy.FailureRateScaleDownStart {
		scale := 1 - ((in.RecentFailureRate - policy.FailureRateScaleDownStart) / maxFloat(0.0001, policy.FailureRateCritical-policy.FailureRateScaleDownStart))
		if scale < 0.3 {
			scale = 0.3
		}
		recommended = int(float64(recommended) * scale)
		reasons = append(reasons, "failure rate above scale-down threshold")
	}

	degraded, unhealthy := countHealthStates(in.HostHealth)
	if degraded > 0 {
		recommended -= maxInt(1, (recommended*policy.DegradedHostPenaltyPercent)/100)
		reasons = append(reasons, "degraded hosts detected")
	}
	if unhealthy > 0 {
		recommended -= maxInt(1, (recommended*policy.UnhealthyHostPenaltyPercent)/100)
		reasons = append(reasons, "unhealthy hosts detected")
	}

	if in.Backlog > 200 && in.RecentFailureRate < policy.FailureRateScaleDownStart && unhealthy == 0 {
		recommended += maxInt(1, recommended/5)
		reasons = append(reasons, "high backlog with healthy fleet; scaling up")
	}

	recommended = clampInt(recommended, policy.MinParallelism, policy.MaxParallelism)
	if len(reasons) == 0 {
		reasons = append(reasons, "steady-state inputs; keeping baseline parallelism")
	}
	return AdaptiveConcurrencyDecision{RecommendedParallelism: recommended, Reasons: reasons}
}

func normalizeAdaptivePolicy(in AdaptiveConcurrencyPolicy) AdaptiveConcurrencyPolicy {
	if in.MinParallelism <= 0 {
		in.MinParallelism = 1
	}
	if in.MaxParallelism < in.MinParallelism {
		in.MaxParallelism = in.MinParallelism
	}
	if in.DefaultParallelism < in.MinParallelism || in.DefaultParallelism > in.MaxParallelism {
		in.DefaultParallelism = in.MinParallelism
	}
	if in.FailureRateScaleDownStart <= 0 {
		in.FailureRateScaleDownStart = 0.10
	}
	if in.FailureRateCritical <= in.FailureRateScaleDownStart {
		in.FailureRateCritical = in.FailureRateScaleDownStart + 0.2
	}
	if in.DegradedHostPenaltyPercent < 0 {
		in.DegradedHostPenaltyPercent = 0
	}
	if in.UnhealthyHostPenaltyPercent < 0 {
		in.UnhealthyHostPenaltyPercent = 0
	}
	in.UpdatedAt = time.Now().UTC()
	return in
}

func countHealthStates(health map[string]string) (int, int) {
	degraded := 0
	unhealthy := 0
	for _, status := range health {
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "degraded":
			degraded++
		case "unhealthy", "failed", "down":
			unhealthy++
		}
	}
	return degraded, unhealthy
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
