package control

import "strings"

type ReleaseBlockerPolicy struct {
	MinAggregateScore           float64 `json:"min_aggregate_score"`
	RequireReadinessPass        bool    `json:"require_readiness_pass"`
	RequireDeprecationLifecycle bool    `json:"require_deprecation_lifecycle"`
	MinSimulationConfidence     float64 `json:"min_simulation_confidence"`
}

type ReleaseBlockerInput struct {
	Readiness            ReadinessReport `json:"readiness"`
	APIDiff              *APIDiffReport  `json:"api_diff,omitempty"`
	SimulationConfidence float64         `json:"simulation_confidence,omitempty"`
	ExternalBlockers     []string        `json:"external_blockers,omitempty"`
}

type ReleaseBlockerReport struct {
	Pass                     bool                 `json:"pass"`
	Policy                   ReleaseBlockerPolicy `json:"policy"`
	ReadinessPass            bool                 `json:"readiness_pass"`
	DeprecationLifecyclePass bool                 `json:"deprecation_lifecycle_pass"`
	AggregateScore           float64              `json:"aggregate_score"`
	SimulationConfidence     float64              `json:"simulation_confidence"`
	CraftsmanshipTier        string               `json:"craftsmanship_tier"` // bronze|silver|gold
	Blockers                 []string             `json:"blockers,omitempty"`
}

func DefaultReleaseBlockerPolicy() ReleaseBlockerPolicy {
	return ReleaseBlockerPolicy{
		MinAggregateScore:           0.90,
		RequireReadinessPass:        true,
		RequireDeprecationLifecycle: true,
		MinSimulationConfidence:     0.85,
	}
}

func EvaluateReleaseBlocker(input ReleaseBlockerInput, policy ReleaseBlockerPolicy) ReleaseBlockerReport {
	if policy.MinAggregateScore <= 0 {
		policy = DefaultReleaseBlockerPolicy()
	}
	if policy.MinSimulationConfidence <= 0 {
		policy.MinSimulationConfidence = 0.85
	}
	blockers := make([]string, 0)
	if policy.RequireReadinessPass && !input.Readiness.Pass {
		blockers = append(blockers, "readiness report did not pass threshold checks")
	}
	if input.Readiness.AggregateScore < policy.MinAggregateScore {
		blockers = append(blockers, "aggregate readiness score below minimum")
	}
	deprecationLifecyclePass := true
	if input.APIDiff != nil {
		deprecationLifecyclePass = input.APIDiff.DeprecationLifecyclePass
		if policy.RequireDeprecationLifecycle && !input.APIDiff.DeprecationLifecyclePass {
			blockers = append(blockers, "api deprecation lifecycle validation failed")
		}
	}
	if input.SimulationConfidence < policy.MinSimulationConfidence {
		blockers = append(blockers, "simulation confidence below minimum threshold")
	}
	for _, item := range input.ExternalBlockers {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		blockers = append(blockers, item)
	}

	return ReleaseBlockerReport{
		Pass:                     len(blockers) == 0,
		Policy:                   policy,
		ReadinessPass:            input.Readiness.Pass,
		DeprecationLifecyclePass: deprecationLifecyclePass,
		AggregateScore:           input.Readiness.AggregateScore,
		SimulationConfidence:     input.SimulationConfidence,
		CraftsmanshipTier:        craftsmanshipTier(input.Readiness.AggregateScore, len(blockers)),
		Blockers:                 blockers,
	}
}

func craftsmanshipTier(score float64, blockerCount int) string {
	if blockerCount == 0 && score >= 0.95 {
		return "gold"
	}
	if blockerCount <= 1 && score >= 0.85 {
		return "silver"
	}
	return "bronze"
}
