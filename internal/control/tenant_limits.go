package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type TenantLimitPolicyInput struct {
	Tenant               string `json:"tenant"`
	RequestsPerMinute    int    `json:"requests_per_minute"`
	MaxConcurrentRuns    int    `json:"max_concurrent_runs"`
	MaxQueueSharePercent int    `json:"max_queue_share_percent"`
	Burst                int    `json:"burst,omitempty"`
}

type TenantLimitPolicy struct {
	ID                   string    `json:"id"`
	Tenant               string    `json:"tenant"`
	RequestsPerMinute    int       `json:"requests_per_minute"`
	MaxConcurrentRuns    int       `json:"max_concurrent_runs"`
	MaxQueueSharePercent int       `json:"max_queue_share_percent"`
	Burst                int       `json:"burst"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type TenantAdmissionInput struct {
	Tenant        string `json:"tenant"`
	RequestedRuns int    `json:"requested_runs"`
	CurrentRuns   int    `json:"current_runs"`
	QueueDepth    int    `json:"queue_depth"`
	TenantQueued  int    `json:"tenant_queued"`
}

type TenantAdmissionDecision struct {
	Allowed     bool   `json:"allowed"`
	Tenant      string `json:"tenant"`
	PolicyID    string `json:"policy_id,omitempty"`
	Reason      string `json:"reason"`
	ThrottleFor int    `json:"throttle_for_seconds,omitempty"`
}

type TenantLimitStore struct {
	mu       sync.RWMutex
	nextID   int64
	policies map[string]*TenantLimitPolicy
	byTenant map[string]string
}

func NewTenantLimitStore() *TenantLimitStore {
	return &TenantLimitStore{policies: map[string]*TenantLimitPolicy{}, byTenant: map[string]string{}}
}

func (s *TenantLimitStore) Upsert(in TenantLimitPolicyInput) (TenantLimitPolicy, error) {
	tenant := strings.ToLower(strings.TrimSpace(in.Tenant))
	if tenant == "" {
		return TenantLimitPolicy{}, errors.New("tenant is required")
	}
	if in.RequestsPerMinute <= 0 || in.MaxConcurrentRuns <= 0 || in.MaxQueueSharePercent <= 0 || in.MaxQueueSharePercent > 100 {
		return TenantLimitPolicy{}, errors.New("invalid tenant limit values")
	}
	burst := in.Burst
	if burst <= 0 {
		burst = in.RequestsPerMinute / 5
		if burst < 1 {
			burst = 1
		}
	}
	item := TenantLimitPolicy{
		Tenant:               tenant,
		RequestsPerMinute:    in.RequestsPerMinute,
		MaxConcurrentRuns:    in.MaxConcurrentRuns,
		MaxQueueSharePercent: in.MaxQueueSharePercent,
		Burst:                burst,
		UpdatedAt:            time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existingID, ok := s.byTenant[tenant]; ok {
		item.ID = existingID
		s.policies[existingID] = &item
		return item, nil
	}
	s.nextID++
	item.ID = "tenant-policy-" + itoa(s.nextID)
	s.policies[item.ID] = &item
	s.byTenant[tenant] = item.ID
	return item, nil
}

func (s *TenantLimitStore) List() []TenantLimitPolicy {
	s.mu.RLock()
	out := make([]TenantLimitPolicy, 0, len(s.policies))
	for _, item := range s.policies {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Tenant < out[j].Tenant })
	return out
}

func (s *TenantLimitStore) byTenantPolicy(tenant string) (TenantLimitPolicy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byTenant[strings.ToLower(strings.TrimSpace(tenant))]
	if !ok {
		return TenantLimitPolicy{}, false
	}
	item, ok := s.policies[id]
	if !ok {
		return TenantLimitPolicy{}, false
	}
	return *item, true
}

func (s *TenantLimitStore) Admit(in TenantAdmissionInput) TenantAdmissionDecision {
	tenant := strings.ToLower(strings.TrimSpace(in.Tenant))
	policy, ok := s.byTenantPolicy(tenant)
	if !ok {
		return TenantAdmissionDecision{Allowed: true, Tenant: tenant, Reason: "no tenant policy configured"}
	}
	if in.CurrentRuns+in.RequestedRuns > policy.MaxConcurrentRuns {
		return TenantAdmissionDecision{Allowed: false, Tenant: tenant, PolicyID: policy.ID, Reason: "max concurrent runs exceeded", ThrottleFor: 60}
	}
	if in.QueueDepth > 0 {
		share := (in.TenantQueued * 100) / in.QueueDepth
		if share > policy.MaxQueueSharePercent {
			return TenantAdmissionDecision{Allowed: false, Tenant: tenant, PolicyID: policy.ID, Reason: "queue share exceeds noisy-neighbor threshold", ThrottleFor: 30}
		}
	}
	return TenantAdmissionDecision{Allowed: true, Tenant: tenant, PolicyID: policy.ID, Reason: "within tenant limits"}
}
