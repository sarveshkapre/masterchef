package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ApprovalStageRule struct {
	Name              string `json:"name"`
	RequiredApprovals int    `json:"required_approvals"`
}

type QuorumApprovalPolicy struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Stages    []ApprovalStageRule `json:"stages"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

type QuorumApprovalPolicyInput struct {
	Name   string              `json:"name"`
	Stages []ApprovalStageRule `json:"stages"`
}

type BreakGlassStatus string

const (
	BreakGlassPending  BreakGlassStatus = "pending"
	BreakGlassActive   BreakGlassStatus = "active"
	BreakGlassRejected BreakGlassStatus = "rejected"
	BreakGlassRevoked  BreakGlassStatus = "revoked"
	BreakGlassExpired  BreakGlassStatus = "expired"
)

type BreakGlassApproval struct {
	Actor      string    `json:"actor"`
	Decision   string    `json:"decision"` // approve|reject
	Comment    string    `json:"comment,omitempty"`
	StageIndex int       `json:"stage_index"`
	StageName  string    `json:"stage_name"`
	CreatedAt  time.Time `json:"created_at"`
}

type BreakGlassRequest struct {
	ID              string               `json:"id"`
	RequestedBy     string               `json:"requested_by"`
	Reason          string               `json:"reason"`
	Scope           string               `json:"scope"`
	PolicyID        string               `json:"policy_id"`
	PolicyName      string               `json:"policy_name"`
	Stages          []ApprovalStageRule  `json:"stages"`
	CurrentStage    int                  `json:"current_stage"`
	Status          BreakGlassStatus     `json:"status"`
	Approvals       []BreakGlassApproval `json:"approvals,omitempty"`
	TTLSeconds      int                  `json:"ttl_seconds"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
	ActivatedAt     *time.Time           `json:"activated_at,omitempty"`
	ExpiresAt       *time.Time           `json:"expires_at,omitempty"`
	RejectedAt      *time.Time           `json:"rejected_at,omitempty"`
	RevokedAt       *time.Time           `json:"revoked_at,omitempty"`
	RejectionReason string               `json:"rejection_reason,omitempty"`
}

type BreakGlassRequestInput struct {
	RequestedBy string `json:"requested_by"`
	Reason      string `json:"reason"`
	Scope       string `json:"scope"`
	PolicyID    string `json:"policy_id"`
	TTLSeconds  int    `json:"ttl_seconds,omitempty"`
}

type AccessApprovalStore struct {
	mu          sync.RWMutex
	nextPolicy  int64
	nextRequest int64
	policies    map[string]*QuorumApprovalPolicy
	requests    map[string]*BreakGlassRequest
}

func NewAccessApprovalStore() *AccessApprovalStore {
	return &AccessApprovalStore{
		policies: map[string]*QuorumApprovalPolicy{},
		requests: map[string]*BreakGlassRequest{},
	}
}

func (s *AccessApprovalStore) CreatePolicy(in QuorumApprovalPolicyInput) (QuorumApprovalPolicy, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return QuorumApprovalPolicy{}, errors.New("name is required")
	}
	stages, err := normalizeApprovalStages(in.Stages)
	if err != nil {
		return QuorumApprovalPolicy{}, err
	}
	now := time.Now().UTC()
	item := QuorumApprovalPolicy{
		Name:      name,
		Stages:    stages,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextPolicy++
	item.ID = "approval-policy-" + itoa(s.nextPolicy)
	s.policies[item.ID] = &item
	return cloneApprovalPolicy(item), nil
}

func (s *AccessApprovalStore) ListPolicies() []QuorumApprovalPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]QuorumApprovalPolicy, 0, len(s.policies))
	for _, item := range s.policies {
		out = append(out, cloneApprovalPolicy(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *AccessApprovalStore) GetPolicy(id string) (QuorumApprovalPolicy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.policies[strings.TrimSpace(id)]
	if !ok {
		return QuorumApprovalPolicy{}, false
	}
	return cloneApprovalPolicy(*item), true
}

func (s *AccessApprovalStore) CreateBreakGlassRequest(in BreakGlassRequestInput) (BreakGlassRequest, error) {
	requestedBy := strings.TrimSpace(in.RequestedBy)
	reason := strings.TrimSpace(in.Reason)
	scope := strings.TrimSpace(in.Scope)
	policyID := strings.TrimSpace(in.PolicyID)
	if requestedBy == "" || reason == "" || scope == "" || policyID == "" {
		return BreakGlassRequest{}, errors.New("requested_by, reason, scope, and policy_id are required")
	}
	ttl := in.TTLSeconds
	if ttl <= 0 {
		ttl = 3600
	}
	if ttl < 300 {
		return BreakGlassRequest{}, errors.New("ttl_seconds must be >= 300")
	}
	if ttl > 86400 {
		return BreakGlassRequest{}, errors.New("ttl_seconds must be <= 86400")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	policy, ok := s.policies[policyID]
	if !ok {
		return BreakGlassRequest{}, errors.New("approval policy not found")
	}
	now := time.Now().UTC()
	s.nextRequest++
	req := BreakGlassRequest{
		ID:           "breakglass-" + itoa(s.nextRequest),
		RequestedBy:  requestedBy,
		Reason:       reason,
		Scope:        scope,
		PolicyID:     policy.ID,
		PolicyName:   policy.Name,
		Stages:       cloneApprovalStages(policy.Stages),
		TTLSeconds:   ttl,
		CurrentStage: 0,
		Status:       BreakGlassPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.requests[req.ID] = &req
	return cloneBreakGlassRequest(req), nil
}

func (s *AccessApprovalStore) ListBreakGlassRequests() []BreakGlassRequest {
	now := time.Now().UTC()
	s.mu.Lock()
	s.expireBreakGlassRequestsLocked(now)
	out := make([]BreakGlassRequest, 0, len(s.requests))
	for _, item := range s.requests {
		out = append(out, cloneBreakGlassRequest(*item))
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *AccessApprovalStore) GetBreakGlassRequest(id string) (BreakGlassRequest, bool) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireBreakGlassRequestsLocked(now)
	item, ok := s.requests[strings.TrimSpace(id)]
	if !ok {
		return BreakGlassRequest{}, false
	}
	return cloneBreakGlassRequest(*item), true
}

func (s *AccessApprovalStore) ApproveBreakGlassRequest(id, actor, comment string) (BreakGlassRequest, error) {
	id = strings.TrimSpace(id)
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return BreakGlassRequest{}, errors.New("actor is required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireBreakGlassRequestsLocked(now)
	req, ok := s.requests[id]
	if !ok {
		return BreakGlassRequest{}, errors.New("break-glass request not found")
	}
	if req.Status != BreakGlassPending {
		return BreakGlassRequest{}, errors.New("break-glass request is not pending approval")
	}
	if req.CurrentStage < 0 || req.CurrentStage >= len(req.Stages) {
		return BreakGlassRequest{}, errors.New("break-glass stage index out of range")
	}
	stage := req.Stages[req.CurrentStage]
	for _, existing := range req.Approvals {
		if existing.StageIndex == req.CurrentStage && strings.EqualFold(existing.Actor, actor) {
			return BreakGlassRequest{}, errors.New("actor has already approved current stage")
		}
	}
	req.Approvals = append(req.Approvals, BreakGlassApproval{
		Actor:      actor,
		Decision:   "approve",
		Comment:    strings.TrimSpace(comment),
		StageIndex: req.CurrentStage,
		StageName:  stage.Name,
		CreatedAt:  now,
	})
	if s.countApprovalsForStage(*req, req.CurrentStage) >= stage.RequiredApprovals {
		req.CurrentStage++
		if req.CurrentStage >= len(req.Stages) {
			req.Status = BreakGlassActive
			activatedAt := now
			expiresAt := now.Add(time.Duration(req.TTLSeconds) * time.Second)
			req.ActivatedAt = &activatedAt
			req.ExpiresAt = &expiresAt
		}
	}
	req.UpdatedAt = now
	return cloneBreakGlassRequest(*req), nil
}

func (s *AccessApprovalStore) RejectBreakGlassRequest(id, actor, comment string) (BreakGlassRequest, error) {
	id = strings.TrimSpace(id)
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return BreakGlassRequest{}, errors.New("actor is required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireBreakGlassRequestsLocked(now)
	req, ok := s.requests[id]
	if !ok {
		return BreakGlassRequest{}, errors.New("break-glass request not found")
	}
	if req.Status != BreakGlassPending {
		return BreakGlassRequest{}, errors.New("break-glass request is not pending approval")
	}
	stageName := "pending"
	if req.CurrentStage >= 0 && req.CurrentStage < len(req.Stages) {
		stageName = req.Stages[req.CurrentStage].Name
	}
	req.Approvals = append(req.Approvals, BreakGlassApproval{
		Actor:      actor,
		Decision:   "reject",
		Comment:    strings.TrimSpace(comment),
		StageIndex: req.CurrentStage,
		StageName:  stageName,
		CreatedAt:  now,
	})
	req.Status = BreakGlassRejected
	rejectedAt := now
	req.RejectedAt = &rejectedAt
	req.RejectionReason = strings.TrimSpace(comment)
	req.UpdatedAt = now
	return cloneBreakGlassRequest(*req), nil
}

func (s *AccessApprovalStore) RevokeBreakGlassRequest(id, actor, reason string) (BreakGlassRequest, error) {
	id = strings.TrimSpace(id)
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return BreakGlassRequest{}, errors.New("actor is required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireBreakGlassRequestsLocked(now)
	req, ok := s.requests[id]
	if !ok {
		return BreakGlassRequest{}, errors.New("break-glass request not found")
	}
	if req.Status != BreakGlassPending && req.Status != BreakGlassActive {
		return BreakGlassRequest{}, errors.New("break-glass request cannot be revoked from current status")
	}
	stageName := "revoke"
	if req.CurrentStage >= 0 && req.CurrentStage < len(req.Stages) {
		stageName = req.Stages[req.CurrentStage].Name
	}
	req.Approvals = append(req.Approvals, BreakGlassApproval{
		Actor:      actor,
		Decision:   "revoke",
		Comment:    strings.TrimSpace(reason),
		StageIndex: req.CurrentStage,
		StageName:  stageName,
		CreatedAt:  now,
	})
	req.Status = BreakGlassRevoked
	revokedAt := now
	req.RevokedAt = &revokedAt
	req.UpdatedAt = now
	return cloneBreakGlassRequest(*req), nil
}

func (s *AccessApprovalStore) countApprovalsForStage(req BreakGlassRequest, stageIndex int) int {
	total := 0
	for _, item := range req.Approvals {
		if item.StageIndex == stageIndex && item.Decision == "approve" {
			total++
		}
	}
	return total
}

func (s *AccessApprovalStore) expireBreakGlassRequestsLocked(now time.Time) {
	for _, req := range s.requests {
		if req.Status != BreakGlassActive || req.ExpiresAt == nil {
			continue
		}
		if !now.Before(*req.ExpiresAt) {
			req.Status = BreakGlassExpired
			req.UpdatedAt = now
		}
	}
}

func normalizeApprovalStages(in []ApprovalStageRule) ([]ApprovalStageRule, error) {
	if len(in) == 0 {
		return nil, errors.New("at least one approval stage is required")
	}
	out := make([]ApprovalStageRule, 0, len(in))
	for i, stage := range in {
		name := strings.TrimSpace(stage.Name)
		if name == "" {
			name = "stage-" + itoa(int64(i+1))
		}
		required := stage.RequiredApprovals
		if required <= 0 {
			required = 1
		}
		out = append(out, ApprovalStageRule{
			Name:              name,
			RequiredApprovals: required,
		})
	}
	return out, nil
}

func cloneApprovalPolicy(in QuorumApprovalPolicy) QuorumApprovalPolicy {
	out := in
	out.Stages = cloneApprovalStages(in.Stages)
	return out
}

func cloneApprovalStages(in []ApprovalStageRule) []ApprovalStageRule {
	out := make([]ApprovalStageRule, len(in))
	copy(out, in)
	return out
}

func cloneBreakGlassRequest(in BreakGlassRequest) BreakGlassRequest {
	out := in
	out.Stages = cloneApprovalStages(in.Stages)
	out.Approvals = append([]BreakGlassApproval{}, in.Approvals...)
	if in.ActivatedAt != nil {
		activatedAt := *in.ActivatedAt
		out.ActivatedAt = &activatedAt
	}
	if in.ExpiresAt != nil {
		expiresAt := *in.ExpiresAt
		out.ExpiresAt = &expiresAt
	}
	if in.RejectedAt != nil {
		rejectedAt := *in.RejectedAt
		out.RejectedAt = &rejectedAt
	}
	if in.RevokedAt != nil {
		revokedAt := *in.RevokedAt
		out.RevokedAt = &revokedAt
	}
	return out
}
