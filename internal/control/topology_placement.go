package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type TopologyPlacementPolicyInput struct {
	Environment   string `json:"environment"`
	Region        string `json:"region,omitempty"`
	Zone          string `json:"zone,omitempty"`
	Cluster       string `json:"cluster,omitempty"`
	FailureDomain string `json:"failure_domain,omitempty"`
	Weight        int    `json:"weight,omitempty"`
	MaxParallel   int    `json:"max_parallel,omitempty"`
}

type TopologyPlacementPolicy struct {
	ID            string    `json:"id"`
	Environment   string    `json:"environment"`
	Region        string    `json:"region,omitempty"`
	Zone          string    `json:"zone,omitempty"`
	Cluster       string    `json:"cluster,omitempty"`
	FailureDomain string    `json:"failure_domain,omitempty"`
	Weight        int       `json:"weight"`
	MaxParallel   int       `json:"max_parallel"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type TopologyPlacementDecisionInput struct {
	Environment   string `json:"environment"`
	Region        string `json:"region,omitempty"`
	Zone          string `json:"zone,omitempty"`
	Cluster       string `json:"cluster,omitempty"`
	FailureDomain string `json:"failure_domain,omitempty"`
	RunKey        string `json:"run_key,omitempty"`
}

type TopologyPlacementDecision struct {
	Environment   string `json:"environment"`
	Region        string `json:"region,omitempty"`
	Zone          string `json:"zone,omitempty"`
	Cluster       string `json:"cluster,omitempty"`
	FailureDomain string `json:"failure_domain,omitempty"`
	PolicyID      string `json:"policy_id,omitempty"`
	MaxParallel   int    `json:"max_parallel"`
	Reason        string `json:"reason"`
}

type TopologyPlacementStore struct {
	mu       sync.RWMutex
	nextID   int64
	policies map[string]*TopologyPlacementPolicy
}

func NewTopologyPlacementStore() *TopologyPlacementStore {
	return &TopologyPlacementStore{policies: map[string]*TopologyPlacementPolicy{}}
}

func (s *TopologyPlacementStore) Upsert(in TopologyPlacementPolicyInput) (TopologyPlacementPolicy, error) {
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	if environment == "" {
		return TopologyPlacementPolicy{}, errors.New("environment is required")
	}
	region := strings.ToLower(strings.TrimSpace(in.Region))
	zone := strings.ToLower(strings.TrimSpace(in.Zone))
	cluster := strings.ToLower(strings.TrimSpace(in.Cluster))
	failureDomain := strings.ToLower(strings.TrimSpace(in.FailureDomain))

	weight := in.Weight
	if weight <= 0 {
		weight = 100
	}
	maxParallel := in.MaxParallel
	if maxParallel <= 0 {
		maxParallel = 25
	}
	item := TopologyPlacementPolicy{
		Environment:   environment,
		Region:        region,
		Zone:          zone,
		Cluster:       cluster,
		FailureDomain: failureDomain,
		Weight:        weight,
		MaxParallel:   maxParallel,
		UpdatedAt:     time.Now().UTC(),
	}

	key := topologyPlacementKey(environment, region, zone, cluster, failureDomain)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.policies[key]; ok {
		item.ID = existing.ID
		s.policies[key] = &item
		return item, nil
	}
	s.nextID++
	item.ID = "topology-placement-policy-" + itoa(s.nextID)
	s.policies[key] = &item
	return item, nil
}

func (s *TopologyPlacementStore) List() []TopologyPlacementPolicy {
	s.mu.RLock()
	out := make([]TopologyPlacementPolicy, 0, len(s.policies))
	for _, item := range s.policies {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Environment == out[j].Environment {
			if out[i].Region == out[j].Region {
				if out[i].Zone == out[j].Zone {
					return out[i].Cluster < out[j].Cluster
				}
				return out[i].Zone < out[j].Zone
			}
			return out[i].Region < out[j].Region
		}
		return out[i].Environment < out[j].Environment
	})
	return out
}

func (s *TopologyPlacementStore) Decide(in TopologyPlacementDecisionInput) TopologyPlacementDecision {
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	region := strings.ToLower(strings.TrimSpace(in.Region))
	zone := strings.ToLower(strings.TrimSpace(in.Zone))
	cluster := strings.ToLower(strings.TrimSpace(in.Cluster))
	failureDomain := strings.ToLower(strings.TrimSpace(in.FailureDomain))

	type scored struct {
		item  TopologyPlacementPolicy
		score int
	}
	candidates := make([]scored, 0, 8)
	for _, item := range s.List() {
		if item.Environment != environment {
			continue
		}
		score := 0
		if item.Region == region && item.Region != "" {
			score += 3
		}
		if item.Zone == zone && item.Zone != "" {
			score += 4
		}
		if item.Cluster == cluster && item.Cluster != "" {
			score += 2
		}
		if item.FailureDomain == failureDomain && item.FailureDomain != "" {
			score += 5
		}
		candidates = append(candidates, scored{item: item, score: score})
	}

	if len(candidates) == 0 {
		return TopologyPlacementDecision{
			Environment:   environment,
			Region:        region,
			Zone:          zone,
			Cluster:       cluster,
			FailureDomain: failureDomain,
			MaxParallel:   10,
			Reason:        "no topology placement policy matched; using conservative default",
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].item.Weight > candidates[j].item.Weight
		}
		return candidates[i].score > candidates[j].score
	})
	picked := candidates[0].item
	return TopologyPlacementDecision{
		Environment:   environment,
		Region:        region,
		Zone:          zone,
		Cluster:       cluster,
		FailureDomain: failureDomain,
		PolicyID:      picked.ID,
		MaxParallel:   picked.MaxParallel,
		Reason:        "matched topology-aware placement policy",
	}
}

func topologyPlacementKey(environment, region, zone, cluster, failureDomain string) string {
	return environment + "|" + region + "|" + zone + "|" + cluster + "|" + failureDomain
}
