package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type WorkspaceIsolationPolicyInput struct {
	Tenant                  string `json:"tenant"`
	Workspace               string `json:"workspace"`
	Environment             string `json:"environment"`
	NetworkSegment          string `json:"network_segment"`
	ComputePool             string `json:"compute_pool"`
	DataScope               string `json:"data_scope"`
	AllowCrossWorkspaceRead bool   `json:"allow_cross_workspace_read,omitempty"`
}

type WorkspaceIsolationPolicy struct {
	ID                      string    `json:"id"`
	Tenant                  string    `json:"tenant"`
	Workspace               string    `json:"workspace"`
	Environment             string    `json:"environment"`
	NetworkSegment          string    `json:"network_segment"`
	ComputePool             string    `json:"compute_pool"`
	DataScope               string    `json:"data_scope"`
	AllowCrossWorkspaceRead bool      `json:"allow_cross_workspace_read"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type WorkspaceIsolationEvaluateInput struct {
	Tenant             string `json:"tenant"`
	Workspace          string `json:"workspace"`
	Environment        string `json:"environment"`
	TargetWorkspace    string `json:"target_workspace,omitempty"`
	RequestedDataScope string `json:"requested_data_scope,omitempty"`
	NetworkSegment     string `json:"network_segment,omitempty"`
	ComputePool        string `json:"compute_pool,omitempty"`
}

type WorkspaceIsolationDecision struct {
	Allowed         bool   `json:"allowed"`
	Tenant          string `json:"tenant"`
	Workspace       string `json:"workspace"`
	Environment     string `json:"environment"`
	PolicyID        string `json:"policy_id,omitempty"`
	IsolationDomain string `json:"isolation_domain,omitempty"`
	Reason          string `json:"reason"`
}

type WorkspaceIsolationStore struct {
	mu       sync.RWMutex
	nextID   int64
	policies map[string]*WorkspaceIsolationPolicy
}

func NewWorkspaceIsolationStore() *WorkspaceIsolationStore {
	return &WorkspaceIsolationStore{policies: map[string]*WorkspaceIsolationPolicy{}}
}

func (s *WorkspaceIsolationStore) Upsert(in WorkspaceIsolationPolicyInput) (WorkspaceIsolationPolicy, error) {
	tenant := strings.ToLower(strings.TrimSpace(in.Tenant))
	workspace := strings.ToLower(strings.TrimSpace(in.Workspace))
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	networkSegment := strings.ToLower(strings.TrimSpace(in.NetworkSegment))
	computePool := strings.ToLower(strings.TrimSpace(in.ComputePool))
	dataScope := strings.ToLower(strings.TrimSpace(in.DataScope))
	if tenant == "" || workspace == "" || environment == "" {
		return WorkspaceIsolationPolicy{}, errors.New("tenant, workspace, and environment are required")
	}
	if networkSegment == "" || computePool == "" {
		return WorkspaceIsolationPolicy{}, errors.New("network_segment and compute_pool are required")
	}
	if dataScope == "" {
		dataScope = tenant + "/" + workspace
	}

	key := workspaceIsolationKey(tenant, workspace, environment)
	item := WorkspaceIsolationPolicy{
		Tenant:                  tenant,
		Workspace:               workspace,
		Environment:             environment,
		NetworkSegment:          networkSegment,
		ComputePool:             computePool,
		DataScope:               dataScope,
		AllowCrossWorkspaceRead: in.AllowCrossWorkspaceRead,
		UpdatedAt:               time.Now().UTC(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.policies[key]; ok {
		item.ID = existing.ID
		s.policies[key] = &item
		return item, nil
	}
	s.nextID++
	item.ID = "workspace-isolation-policy-" + itoa(s.nextID)
	s.policies[key] = &item
	return item, nil
}

func (s *WorkspaceIsolationStore) List() []WorkspaceIsolationPolicy {
	s.mu.RLock()
	out := make([]WorkspaceIsolationPolicy, 0, len(s.policies))
	for _, item := range s.policies {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tenant == out[j].Tenant {
			if out[i].Workspace == out[j].Workspace {
				return out[i].Environment < out[j].Environment
			}
			return out[i].Workspace < out[j].Workspace
		}
		return out[i].Tenant < out[j].Tenant
	})
	return out
}

func (s *WorkspaceIsolationStore) Evaluate(in WorkspaceIsolationEvaluateInput) WorkspaceIsolationDecision {
	tenant := strings.ToLower(strings.TrimSpace(in.Tenant))
	workspace := strings.ToLower(strings.TrimSpace(in.Workspace))
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	key := workspaceIsolationKey(tenant, workspace, environment)

	s.mu.RLock()
	policy, ok := s.policies[key]
	s.mu.RUnlock()
	if !ok {
		return WorkspaceIsolationDecision{
			Allowed:     false,
			Tenant:      tenant,
			Workspace:   workspace,
			Environment: environment,
			Reason:      "no workspace isolation policy configured",
		}
	}

	targetWorkspace := strings.ToLower(strings.TrimSpace(in.TargetWorkspace))
	if targetWorkspace == "" {
		targetWorkspace = workspace
	}
	if targetWorkspace != workspace && !policy.AllowCrossWorkspaceRead {
		return WorkspaceIsolationDecision{
			Allowed:         false,
			Tenant:          tenant,
			Workspace:       workspace,
			Environment:     environment,
			PolicyID:        policy.ID,
			IsolationDomain: policy.Tenant + ":" + policy.Workspace + ":" + policy.Environment,
			Reason:          "cross-workspace access denied by policy",
		}
	}
	if seg := strings.ToLower(strings.TrimSpace(in.NetworkSegment)); seg != "" && seg != policy.NetworkSegment {
		return WorkspaceIsolationDecision{
			Allowed:         false,
			Tenant:          tenant,
			Workspace:       workspace,
			Environment:     environment,
			PolicyID:        policy.ID,
			IsolationDomain: policy.Tenant + ":" + policy.Workspace + ":" + policy.Environment,
			Reason:          "network segment does not match workspace isolation policy",
		}
	}
	if pool := strings.ToLower(strings.TrimSpace(in.ComputePool)); pool != "" && pool != policy.ComputePool {
		return WorkspaceIsolationDecision{
			Allowed:         false,
			Tenant:          tenant,
			Workspace:       workspace,
			Environment:     environment,
			PolicyID:        policy.ID,
			IsolationDomain: policy.Tenant + ":" + policy.Workspace + ":" + policy.Environment,
			Reason:          "compute pool does not match workspace isolation policy",
		}
	}
	if scope := strings.ToLower(strings.TrimSpace(in.RequestedDataScope)); scope != "" && scope != policy.DataScope {
		return WorkspaceIsolationDecision{
			Allowed:         false,
			Tenant:          tenant,
			Workspace:       workspace,
			Environment:     environment,
			PolicyID:        policy.ID,
			IsolationDomain: policy.Tenant + ":" + policy.Workspace + ":" + policy.Environment,
			Reason:          "requested data scope is outside workspace boundary",
		}
	}

	return WorkspaceIsolationDecision{
		Allowed:         true,
		Tenant:          tenant,
		Workspace:       workspace,
		Environment:     environment,
		PolicyID:        policy.ID,
		IsolationDomain: policy.Tenant + ":" + policy.Workspace + ":" + policy.Environment,
		Reason:          "request is within workspace isolation boundary",
	}
}

func workspaceIsolationKey(tenant, workspace, environment string) string {
	return tenant + "|" + workspace + "|" + environment
}
