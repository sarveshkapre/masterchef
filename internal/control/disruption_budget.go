package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type DisruptionBudget struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Scope          string    `json:"scope,omitempty"`
	MaxUnavailable int       `json:"max_unavailable"`
	MinHealthyPct  int       `json:"min_healthy_pct"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type DisruptionBudgetInput struct {
	Name           string `json:"name"`
	Scope          string `json:"scope,omitempty"`
	MaxUnavailable int    `json:"max_unavailable,omitempty"`
	MinHealthyPct  int    `json:"min_healthy_pct,omitempty"`
}

type DisruptionBudgetEvaluation struct {
	BudgetID              string `json:"budget_id"`
	Allowed               bool   `json:"allowed"`
	Reason                string `json:"reason,omitempty"`
	TotalTargets          int    `json:"total_targets"`
	RequestedDisruptions  int    `json:"requested_disruptions"`
	RemainingHealthyCount int    `json:"remaining_healthy_count"`
	HealthyPctAfter       int    `json:"healthy_pct_after"`
}

type DisruptionBudgetStore struct {
	mu      sync.RWMutex
	nextID  int64
	budgets map[string]*DisruptionBudget
}

func NewDisruptionBudgetStore() *DisruptionBudgetStore {
	return &DisruptionBudgetStore{budgets: map[string]*DisruptionBudget{}}
}

func (s *DisruptionBudgetStore) Create(in DisruptionBudgetInput) (DisruptionBudget, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return DisruptionBudget{}, errors.New("name is required")
	}
	if in.MaxUnavailable <= 0 {
		in.MaxUnavailable = 1
	}
	if in.MinHealthyPct <= 0 {
		in.MinHealthyPct = 90
	}
	if in.MinHealthyPct > 100 {
		in.MinHealthyPct = 100
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item := &DisruptionBudget{
		ID:             "budget-" + itoa(s.nextID),
		Name:           name,
		Scope:          strings.TrimSpace(in.Scope),
		MaxUnavailable: in.MaxUnavailable,
		MinHealthyPct:  in.MinHealthyPct,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.budgets[item.ID] = item
	return cloneDisruptionBudget(*item), nil
}

func (s *DisruptionBudgetStore) List() []DisruptionBudget {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]DisruptionBudget, 0, len(s.budgets))
	for _, item := range s.budgets {
		out = append(out, cloneDisruptionBudget(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *DisruptionBudgetStore) Get(id string) (DisruptionBudget, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.budgets[strings.TrimSpace(id)]
	if !ok {
		return DisruptionBudget{}, false
	}
	return cloneDisruptionBudget(*item), true
}

func EvaluateDisruptionBudget(budget DisruptionBudget, totalTargets, requestedDisruptions int) DisruptionBudgetEvaluation {
	if totalTargets < 0 {
		totalTargets = 0
	}
	if requestedDisruptions < 0 {
		requestedDisruptions = 0
	}
	remaining := totalTargets - requestedDisruptions
	if remaining < 0 {
		remaining = 0
	}
	healthyPct := 100
	if totalTargets > 0 {
		healthyPct = (remaining * 100) / totalTargets
	}
	allowed := true
	reason := ""
	if requestedDisruptions > budget.MaxUnavailable {
		allowed = false
		reason = "requested disruptions exceed max_unavailable"
	}
	if healthyPct < budget.MinHealthyPct {
		allowed = false
		if reason == "" {
			reason = "healthy percentage after disruption below min_healthy_pct"
		} else {
			reason += "; healthy percentage after disruption below min_healthy_pct"
		}
	}
	return DisruptionBudgetEvaluation{
		BudgetID:              budget.ID,
		Allowed:               allowed,
		Reason:                reason,
		TotalTargets:          totalTargets,
		RequestedDisruptions:  requestedDisruptions,
		RemainingHealthyCount: remaining,
		HealthyPctAfter:       healthyPct,
	}
}

func cloneDisruptionBudget(in DisruptionBudget) DisruptionBudget {
	return in
}
