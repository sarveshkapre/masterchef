package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type DependencyUpdatePolicy struct {
	Enabled                   bool      `json:"enabled"`
	MaxUpdatesPerDay          int       `json:"max_updates_per_day"`
	RequireCompatibilityCheck bool      `json:"require_compatibility_check"`
	RequirePerformanceCheck   bool      `json:"require_performance_check"`
	AllowedEcosystems         []string  `json:"allowed_ecosystems,omitempty"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

type DependencyUpdateInput struct {
	Ecosystem      string `json:"ecosystem"`
	Package        string `json:"package"`
	CurrentVersion string `json:"current_version"`
	TargetVersion  string `json:"target_version"`
	Reason         string `json:"reason,omitempty"`
}

type DependencyUpdateProposal struct {
	ID                    string    `json:"id"`
	Ecosystem             string    `json:"ecosystem"`
	Package               string    `json:"package"`
	CurrentVersion        string    `json:"current_version"`
	TargetVersion         string    `json:"target_version"`
	Reason                string    `json:"reason,omitempty"`
	CompatibilityChecked  bool      `json:"compatibility_checked"`
	CompatibilityPassed   bool      `json:"compatibility_passed"`
	PerformanceChecked    bool      `json:"performance_checked"`
	PerformanceRegression bool      `json:"performance_regression"`
	PerformanceDeltaPct   float64   `json:"performance_delta_pct"`
	ReadyForMerge         bool      `json:"ready_for_merge"`
	BlockedReasons        []string  `json:"blocked_reasons,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type DependencyUpdateEvaluationInput struct {
	UpdateID             string  `json:"update_id"`
	CompatibilityChecked bool    `json:"compatibility_checked"`
	CompatibilityPassed  bool    `json:"compatibility_passed"`
	PerformanceChecked   bool    `json:"performance_checked"`
	PerformanceDeltaPct  float64 `json:"performance_delta_pct"`
}

type DependencyUpdateStore struct {
	mu      sync.RWMutex
	nextID  int64
	policy  DependencyUpdatePolicy
	updates map[string]*DependencyUpdateProposal
}

func NewDependencyUpdateStore() *DependencyUpdateStore {
	return &DependencyUpdateStore{
		policy: DependencyUpdatePolicy{
			Enabled:                   true,
			MaxUpdatesPerDay:          20,
			RequireCompatibilityCheck: true,
			RequirePerformanceCheck:   true,
			AllowedEcosystems:         []string{"go", "npm", "pypi", "maven"},
			UpdatedAt:                 time.Now().UTC(),
		},
		updates: map[string]*DependencyUpdateProposal{},
	}
}

func (s *DependencyUpdateStore) Policy() DependencyUpdatePolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneDependencyUpdatePolicy(s.policy)
}

func (s *DependencyUpdateStore) SetPolicy(policy DependencyUpdatePolicy) (DependencyUpdatePolicy, error) {
	if policy.MaxUpdatesPerDay <= 0 {
		policy.MaxUpdatesPerDay = 20
	}
	policy.AllowedEcosystems = normalizeStringSlice(policy.AllowedEcosystems)
	policy.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	s.policy = policy
	s.mu.Unlock()
	return cloneDependencyUpdatePolicy(policy), nil
}

func (s *DependencyUpdateStore) Propose(in DependencyUpdateInput) (DependencyUpdateProposal, error) {
	eco := strings.ToLower(strings.TrimSpace(in.Ecosystem))
	pkg := strings.TrimSpace(in.Package)
	current := strings.TrimSpace(in.CurrentVersion)
	target := strings.TrimSpace(in.TargetVersion)
	reason := strings.TrimSpace(in.Reason)
	if eco == "" || pkg == "" || current == "" || target == "" {
		return DependencyUpdateProposal{}, errors.New("ecosystem, package, current_version, and target_version are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	policy := s.policy
	if !policy.Enabled {
		return DependencyUpdateProposal{}, errors.New("dependency update bot is disabled")
	}
	if len(policy.AllowedEcosystems) > 0 && !sliceContains(policy.AllowedEcosystems, eco) {
		return DependencyUpdateProposal{}, errors.New("ecosystem is not allowed by policy")
	}
	if count := s.createdTodayLocked(); count >= policy.MaxUpdatesPerDay {
		return DependencyUpdateProposal{}, errors.New("daily update proposal limit reached")
	}

	now := time.Now().UTC()
	s.nextID++
	item := DependencyUpdateProposal{
		ID:             "dep-update-" + itoa(s.nextID),
		Ecosystem:      eco,
		Package:        pkg,
		CurrentVersion: current,
		TargetVersion:  target,
		Reason:         reason,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	item = finalizeDependencyUpdateProposal(item, policy)
	s.updates[item.ID] = &item
	return cloneDependencyUpdateProposal(item), nil
}

func (s *DependencyUpdateStore) Evaluate(in DependencyUpdateEvaluationInput) (DependencyUpdateProposal, error) {
	id := strings.TrimSpace(in.UpdateID)
	if id == "" {
		return DependencyUpdateProposal{}, errors.New("update_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.updates[id]
	if !ok {
		return DependencyUpdateProposal{}, errors.New("dependency update proposal not found")
	}
	item.CompatibilityChecked = in.CompatibilityChecked
	item.CompatibilityPassed = in.CompatibilityPassed
	item.PerformanceChecked = in.PerformanceChecked
	item.PerformanceDeltaPct = in.PerformanceDeltaPct
	item.PerformanceRegression = in.PerformanceChecked && in.PerformanceDeltaPct > 5
	item.UpdatedAt = time.Now().UTC()
	*item = finalizeDependencyUpdateProposal(*item, s.policy)
	return cloneDependencyUpdateProposal(*item), nil
}

func (s *DependencyUpdateStore) List(limit int) []DependencyUpdateProposal {
	if limit <= 0 {
		limit = 100
	}
	s.mu.RLock()
	out := make([]DependencyUpdateProposal, 0, len(s.updates))
	for _, item := range s.updates {
		out = append(out, cloneDependencyUpdateProposal(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *DependencyUpdateStore) Get(id string) (DependencyUpdateProposal, bool) {
	s.mu.RLock()
	item, ok := s.updates[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return DependencyUpdateProposal{}, false
	}
	return cloneDependencyUpdateProposal(*item), true
}

func (s *DependencyUpdateStore) createdTodayLocked() int {
	now := time.Now().UTC()
	y, m, d := now.Date()
	count := 0
	for _, item := range s.updates {
		iy, im, id := item.CreatedAt.UTC().Date()
		if iy == y && im == m && id == d {
			count++
		}
	}
	return count
}

func finalizeDependencyUpdateProposal(item DependencyUpdateProposal, policy DependencyUpdatePolicy) DependencyUpdateProposal {
	reasons := make([]string, 0, 3)
	if policy.RequireCompatibilityCheck {
		if !item.CompatibilityChecked {
			reasons = append(reasons, "compatibility verification is required")
		} else if !item.CompatibilityPassed {
			reasons = append(reasons, "compatibility verification failed")
		}
	}
	if policy.RequirePerformanceCheck {
		if !item.PerformanceChecked {
			reasons = append(reasons, "performance verification is required")
		} else if item.PerformanceRegression {
			reasons = append(reasons, "performance regression exceeds threshold")
		}
	}
	item.BlockedReasons = reasons
	item.ReadyForMerge = len(reasons) == 0
	return item
}

func cloneDependencyUpdatePolicy(in DependencyUpdatePolicy) DependencyUpdatePolicy {
	out := in
	out.AllowedEcosystems = append([]string{}, in.AllowedEcosystems...)
	return out
}

func cloneDependencyUpdateProposal(in DependencyUpdateProposal) DependencyUpdateProposal {
	out := in
	out.BlockedReasons = append([]string{}, in.BlockedReasons...)
	return out
}
