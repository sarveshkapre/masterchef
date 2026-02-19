package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type CostSchedulingPolicyInput struct {
	Environment           string  `json:"environment"`
	MaxCostPerRun         float64 `json:"max_cost_per_run"`
	MaxHourlyBudget       float64 `json:"max_hourly_budget"`
	OffPeakCostMultiplier float64 `json:"off_peak_cost_multiplier,omitempty"`
	ThrottleAbovePercent  int     `json:"throttle_above_percent,omitempty"`
}

type CostSchedulingPolicy struct {
	ID                    string    `json:"id"`
	Environment           string    `json:"environment"`
	MaxCostPerRun         float64   `json:"max_cost_per_run"`
	MaxHourlyBudget       float64   `json:"max_hourly_budget"`
	OffPeakCostMultiplier float64   `json:"off_peak_cost_multiplier"`
	ThrottleAbovePercent  int       `json:"throttle_above_percent"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type CostSchedulingAdmissionInput struct {
	Environment   string  `json:"environment"`
	EstimatedCost float64 `json:"estimated_cost"`
	HourlySpend   float64 `json:"hourly_spend"`
	QueueDepth    int     `json:"queue_depth"`
	Priority      string  `json:"priority,omitempty"`
	OffPeak       bool    `json:"off_peak,omitempty"`
}

type CostSchedulingDecision struct {
	Allowed             bool    `json:"allowed"`
	Environment         string  `json:"environment"`
	PolicyID            string  `json:"policy_id,omitempty"`
	Reason              string  `json:"reason"`
	EffectiveCost       float64 `json:"effective_cost"`
	BudgetUtilization   float64 `json:"budget_utilization_percent"`
	ThrottleSeconds     int     `json:"throttle_seconds,omitempty"`
	RecommendedPriority string  `json:"recommended_priority,omitempty"`
}

type CostSchedulingStore struct {
	mu       sync.RWMutex
	nextID   int64
	policies map[string]*CostSchedulingPolicy
	byEnv    map[string]string
}

func NewCostSchedulingStore() *CostSchedulingStore {
	return &CostSchedulingStore{
		policies: map[string]*CostSchedulingPolicy{},
		byEnv:    map[string]string{},
	}
}

func (s *CostSchedulingStore) Upsert(in CostSchedulingPolicyInput) (CostSchedulingPolicy, error) {
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	if environment == "" {
		return CostSchedulingPolicy{}, errors.New("environment is required")
	}
	if in.MaxCostPerRun <= 0 || in.MaxHourlyBudget <= 0 {
		return CostSchedulingPolicy{}, errors.New("max_cost_per_run and max_hourly_budget must be greater than zero")
	}
	multiplier := in.OffPeakCostMultiplier
	if multiplier <= 0 {
		multiplier = 0.75
	}
	throttle := in.ThrottleAbovePercent
	if throttle <= 0 || throttle > 100 {
		throttle = 85
	}
	item := CostSchedulingPolicy{
		Environment:           environment,
		MaxCostPerRun:         in.MaxCostPerRun,
		MaxHourlyBudget:       in.MaxHourlyBudget,
		OffPeakCostMultiplier: multiplier,
		ThrottleAbovePercent:  throttle,
		UpdatedAt:             time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existingID, ok := s.byEnv[environment]; ok {
		item.ID = existingID
		s.policies[existingID] = &item
		return item, nil
	}
	s.nextID++
	item.ID = "cost-scheduling-policy-" + itoa(s.nextID)
	s.policies[item.ID] = &item
	s.byEnv[environment] = item.ID
	return item, nil
}

func (s *CostSchedulingStore) List() []CostSchedulingPolicy {
	s.mu.RLock()
	out := make([]CostSchedulingPolicy, 0, len(s.policies))
	for _, item := range s.policies {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Environment < out[j].Environment })
	return out
}

func (s *CostSchedulingStore) policyByEnv(environment string) (CostSchedulingPolicy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byEnv[strings.ToLower(strings.TrimSpace(environment))]
	if !ok {
		return CostSchedulingPolicy{}, false
	}
	item, ok := s.policies[id]
	if !ok {
		return CostSchedulingPolicy{}, false
	}
	return *item, true
}

func (s *CostSchedulingStore) Admit(in CostSchedulingAdmissionInput) (CostSchedulingDecision, error) {
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	if environment == "" {
		return CostSchedulingDecision{}, errors.New("environment is required")
	}
	if in.EstimatedCost < 0 || in.HourlySpend < 0 || in.QueueDepth < 0 {
		return CostSchedulingDecision{}, errors.New("estimated_cost, hourly_spend, and queue_depth must be non-negative")
	}
	policy, ok := s.policyByEnv(environment)
	if !ok {
		return CostSchedulingDecision{
			Allowed:           true,
			Environment:       environment,
			Reason:            "no cost policy configured",
			EffectiveCost:     in.EstimatedCost,
			BudgetUtilization: 0,
		}, nil
	}
	effective := in.EstimatedCost
	if in.OffPeak {
		effective = effective * policy.OffPeakCostMultiplier
	}
	utilization := 0.0
	if policy.MaxHourlyBudget > 0 {
		utilization = ((in.HourlySpend + effective) / policy.MaxHourlyBudget) * 100
	}
	recommendedPriority := strings.ToLower(strings.TrimSpace(in.Priority))
	if recommendedPriority == "" {
		recommendedPriority = "normal"
	}
	if utilization > float64(policy.ThrottleAbovePercent) || (effective > policy.MaxCostPerRun*0.75 && in.QueueDepth > 50) {
		recommendedPriority = "low"
	}
	if effective > policy.MaxCostPerRun {
		return CostSchedulingDecision{
			Allowed:             false,
			Environment:         environment,
			PolicyID:            policy.ID,
			Reason:              "estimated cost exceeds max_cost_per_run",
			EffectiveCost:       effective,
			BudgetUtilization:   utilization,
			ThrottleSeconds:     300,
			RecommendedPriority: recommendedPriority,
		}, nil
	}
	if utilization > 100 {
		return CostSchedulingDecision{
			Allowed:             false,
			Environment:         environment,
			PolicyID:            policy.ID,
			Reason:              "hourly budget exceeded",
			EffectiveCost:       effective,
			BudgetUtilization:   utilization,
			ThrottleSeconds:     600,
			RecommendedPriority: "low",
		}, nil
	}
	if utilization > float64(policy.ThrottleAbovePercent) {
		return CostSchedulingDecision{
			Allowed:             false,
			Environment:         environment,
			PolicyID:            policy.ID,
			Reason:              "budget utilization above throttle threshold",
			EffectiveCost:       effective,
			BudgetUtilization:   utilization,
			ThrottleSeconds:     120,
			RecommendedPriority: "low",
		}, nil
	}
	return CostSchedulingDecision{
		Allowed:             true,
		Environment:         environment,
		PolicyID:            policy.ID,
		Reason:              "within cost policy",
		EffectiveCost:       effective,
		BudgetUtilization:   utilization,
		RecommendedPriority: recommendedPriority,
	}, nil
}
