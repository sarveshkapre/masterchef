package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type RebootPolicyInput struct {
	Environment          string   `json:"environment"`
	MaxConcurrentReboots int      `json:"max_concurrent_reboots"`
	MinHealthyPercent    int      `json:"min_healthy_percent"`
	DependencyOrder      []string `json:"dependency_order,omitempty"`
}

type RebootPolicy struct {
	ID                   string    `json:"id"`
	Environment          string    `json:"environment"`
	MaxConcurrentReboots int       `json:"max_concurrent_reboots"`
	MinHealthyPercent    int       `json:"min_healthy_percent"`
	DependencyOrder      []string  `json:"dependency_order,omitempty"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type RebootHost struct {
	ID            string `json:"id"`
	Role          string `json:"role"`
	FailureDomain string `json:"failure_domain,omitempty"`
	Healthy       bool   `json:"healthy"`
}

type RebootPlanInput struct {
	Environment string       `json:"environment"`
	Hosts       []RebootHost `json:"hosts"`
}

type RebootWave struct {
	Index  int      `json:"index"`
	Role   string   `json:"role"`
	Hosts  []string `json:"hosts"`
	Reason string   `json:"reason"`
}

type RebootPlan struct {
	Allowed        bool         `json:"allowed"`
	Environment    string       `json:"environment"`
	PolicyID       string       `json:"policy_id,omitempty"`
	HealthyPercent int          `json:"healthy_percent"`
	BlockedReason  string       `json:"blocked_reason,omitempty"`
	Waves          []RebootWave `json:"waves,omitempty"`
}

type RebootOrchestrationStore struct {
	mu       sync.RWMutex
	nextID   int64
	policies map[string]*RebootPolicy
}

func NewRebootOrchestrationStore() *RebootOrchestrationStore {
	return &RebootOrchestrationStore{policies: map[string]*RebootPolicy{}}
}

func (s *RebootOrchestrationStore) UpsertPolicy(in RebootPolicyInput) (RebootPolicy, error) {
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	if environment == "" {
		return RebootPolicy{}, errors.New("environment is required")
	}
	maxConcurrent := in.MaxConcurrentReboots
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	minHealthy := in.MinHealthyPercent
	if minHealthy <= 0 || minHealthy > 100 {
		minHealthy = 80
	}
	item := RebootPolicy{
		Environment:          environment,
		MaxConcurrentReboots: maxConcurrent,
		MinHealthyPercent:    minHealthy,
		DependencyOrder:      normalizeOrderedRoleList(in.DependencyOrder),
		UpdatedAt:            time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.policies[environment]; ok {
		item.ID = existing.ID
		s.policies[environment] = &item
		return item, nil
	}
	s.nextID++
	item.ID = "reboot-policy-" + itoa(s.nextID)
	s.policies[environment] = &item
	return item, nil
}

func (s *RebootOrchestrationStore) ListPolicies() []RebootPolicy {
	s.mu.RLock()
	out := make([]RebootPolicy, 0, len(s.policies))
	for _, item := range s.policies {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Environment < out[j].Environment })
	return out
}

func (s *RebootOrchestrationStore) Plan(in RebootPlanInput) RebootPlan {
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	if environment == "" {
		return RebootPlan{Allowed: false, Environment: environment, BlockedReason: "environment is required"}
	}
	if len(in.Hosts) == 0 {
		return RebootPlan{Allowed: false, Environment: environment, BlockedReason: "hosts are required"}
	}
	policy := RebootPolicy{
		Environment:          environment,
		MaxConcurrentReboots: 1,
		MinHealthyPercent:    80,
	}
	if item, ok := s.policies[environment]; ok {
		policy = *item
	}

	total := len(in.Hosts)
	healthy := 0
	for _, host := range in.Hosts {
		if host.Healthy {
			healthy++
		}
	}
	healthyPercent := int(float64(healthy) * 100 / float64(total))
	if healthyPercent < policy.MinHealthyPercent {
		return RebootPlan{
			Allowed:        false,
			Environment:    environment,
			PolicyID:       policy.ID,
			HealthyPercent: healthyPercent,
			BlockedReason:  "fleet healthy percentage below reboot safety threshold",
		}
	}

	hostsByRole := map[string][]RebootHost{}
	roleOrder := append([]string{}, policy.DependencyOrder...)
	seenRole := map[string]struct{}{}
	for _, role := range roleOrder {
		seenRole[role] = struct{}{}
	}
	for _, host := range in.Hosts {
		role := strings.ToLower(strings.TrimSpace(host.Role))
		if role == "" {
			role = "default"
		}
		hostsByRole[role] = append(hostsByRole[role], host)
		if _, ok := seenRole[role]; !ok {
			roleOrder = append(roleOrder, role)
			seenRole[role] = struct{}{}
		}
	}

	waves := make([]RebootWave, 0, total)
	waveIndex := 1
	for _, role := range roleOrder {
		roleHosts := hostsByRole[role]
		if len(roleHosts) == 0 {
			continue
		}
		sort.Slice(roleHosts, func(i, j int) bool {
			a := strings.ToLower(strings.TrimSpace(roleHosts[i].FailureDomain))
			b := strings.ToLower(strings.TrimSpace(roleHosts[j].FailureDomain))
			if a == b {
				return roleHosts[i].ID < roleHosts[j].ID
			}
			return a < b
		})
		for i := 0; i < len(roleHosts); i += policy.MaxConcurrentReboots {
			end := i + policy.MaxConcurrentReboots
			if end > len(roleHosts) {
				end = len(roleHosts)
			}
			hostIDs := make([]string, 0, end-i)
			for _, item := range roleHosts[i:end] {
				hostIDs = append(hostIDs, item.ID)
			}
			waves = append(waves, RebootWave{
				Index:  waveIndex,
				Role:   role,
				Hosts:  hostIDs,
				Reason: "role-ordered reboot wave with max concurrency guard",
			})
			waveIndex++
		}
	}
	return RebootPlan{
		Allowed:        true,
		Environment:    environment,
		PolicyID:       policy.ID,
		HealthyPercent: healthyPercent,
		Waves:          waves,
	}
}

func normalizeOrderedRoleList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, raw := range values {
		item := strings.ToLower(strings.TrimSpace(raw))
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
