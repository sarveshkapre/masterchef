package control

import (
	"errors"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"
)

type SchedulerPartitionRuleInput struct {
	Tenant      string `json:"tenant"`
	Environment string `json:"environment,omitempty"`
	Region      string `json:"region,omitempty"`
	Shard       string `json:"shard"`
	Weight      int    `json:"weight,omitempty"`
	MaxParallel int    `json:"max_parallel,omitempty"`
}

type SchedulerPartitionRule struct {
	ID          string    `json:"id"`
	Tenant      string    `json:"tenant"`
	Environment string    `json:"environment,omitempty"`
	Region      string    `json:"region,omitempty"`
	Shard       string    `json:"shard"`
	Weight      int       `json:"weight"`
	MaxParallel int       `json:"max_parallel"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SchedulerPartitionDecisionInput struct {
	Tenant      string `json:"tenant"`
	Environment string `json:"environment,omitempty"`
	Region      string `json:"region,omitempty"`
	WorkloadKey string `json:"workload_key,omitempty"`
}

type SchedulerPartitionDecision struct {
	Tenant      string `json:"tenant"`
	Environment string `json:"environment,omitempty"`
	Region      string `json:"region,omitempty"`
	Shard       string `json:"shard"`
	MaxParallel int    `json:"max_parallel"`
	RuleID      string `json:"rule_id,omitempty"`
	Reason      string `json:"reason"`
}

type SchedulerPartitionStore struct {
	mu    sync.RWMutex
	next  int64
	rules map[string]*SchedulerPartitionRule
}

func NewSchedulerPartitionStore() *SchedulerPartitionStore {
	return &SchedulerPartitionStore{rules: map[string]*SchedulerPartitionRule{}}
}

func (s *SchedulerPartitionStore) Upsert(in SchedulerPartitionRuleInput) (SchedulerPartitionRule, error) {
	tenant := strings.ToLower(strings.TrimSpace(in.Tenant))
	shard := strings.ToLower(strings.TrimSpace(in.Shard))
	if tenant == "" || shard == "" {
		return SchedulerPartitionRule{}, errors.New("tenant and shard are required")
	}
	env := strings.ToLower(strings.TrimSpace(in.Environment))
	region := strings.ToLower(strings.TrimSpace(in.Region))
	weight := in.Weight
	if weight <= 0 {
		weight = 100
	}
	maxParallel := in.MaxParallel
	if maxParallel <= 0 {
		maxParallel = 50
	}
	item := SchedulerPartitionRule{
		Tenant:      tenant,
		Environment: env,
		Region:      region,
		Shard:       shard,
		Weight:      weight,
		MaxParallel: maxParallel,
		UpdatedAt:   time.Now().UTC(),
	}
	s.mu.Lock()
	s.next++
	item.ID = "partition-rule-" + itoa(s.next)
	s.rules[item.ID] = &item
	s.mu.Unlock()
	return item, nil
}

func (s *SchedulerPartitionStore) List() []SchedulerPartitionRule {
	s.mu.RLock()
	out := make([]SchedulerPartitionRule, 0, len(s.rules))
	for _, item := range s.rules {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tenant == out[j].Tenant {
			return out[i].Shard < out[j].Shard
		}
		return out[i].Tenant < out[j].Tenant
	})
	return out
}

func (s *SchedulerPartitionStore) Get(id string) (SchedulerPartitionRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.rules[strings.TrimSpace(id)]
	if !ok {
		return SchedulerPartitionRule{}, false
	}
	return *item, true
}

func (s *SchedulerPartitionStore) Decide(in SchedulerPartitionDecisionInput) SchedulerPartitionDecision {
	tenant := strings.ToLower(strings.TrimSpace(in.Tenant))
	env := strings.ToLower(strings.TrimSpace(in.Environment))
	region := strings.ToLower(strings.TrimSpace(in.Region))
	key := strings.TrimSpace(in.WorkloadKey)
	if key == "" {
		key = tenant + ":" + env + ":" + region
	}

	rules := s.List()
	candidates := make([]SchedulerPartitionRule, 0)
	for _, rule := range rules {
		if rule.Tenant != tenant {
			continue
		}
		if rule.Environment != "" && rule.Environment != env {
			continue
		}
		if rule.Region != "" && rule.Region != region {
			continue
		}
		candidates = append(candidates, rule)
	}
	if len(candidates) == 0 {
		fallback := deterministicShard(key, 8)
		return SchedulerPartitionDecision{
			Tenant:      tenant,
			Environment: env,
			Region:      region,
			Shard:       fallback,
			MaxParallel: 25,
			Reason:      "default deterministic shard assignment",
		}
	}
	idx := deterministicIndex(key, len(candidates))
	picked := candidates[idx]
	return SchedulerPartitionDecision{
		Tenant:      tenant,
		Environment: env,
		Region:      region,
		Shard:       picked.Shard,
		MaxParallel: picked.MaxParallel,
		RuleID:      picked.ID,
		Reason:      "matched tenant partition rule",
	}
}

func deterministicIndex(key string, mod int) int {
	if mod <= 1 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % uint32(mod))
}

func deterministicShard(key string, count int) string {
	if count <= 0 {
		count = 8
	}
	idx := deterministicIndex(key, count)
	return "shard-" + itoa(int64(idx+1))
}
