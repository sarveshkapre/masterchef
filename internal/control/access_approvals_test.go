package control

import (
	"testing"
	"time"
)

func TestBreakGlassQuorumWorkflow(t *testing.T) {
	store := NewAccessApprovalStore()
	policy, err := store.CreatePolicy(QuorumApprovalPolicyInput{
		Name: "prod-sensitive",
		Stages: []ApprovalStageRule{
			{Name: "peer-review", RequiredApprovals: 1},
			{Name: "security-approval", RequiredApprovals: 2},
		},
	})
	if err != nil {
		t.Fatalf("create policy failed: %v", err)
	}
	if policy.ID == "" || len(policy.Stages) != 2 {
		t.Fatalf("unexpected policy: %+v", policy)
	}

	req, err := store.CreateBreakGlassRequest(BreakGlassRequestInput{
		RequestedBy: "oncall-sre",
		Reason:      "critical prod recovery",
		Scope:       "cluster/prod-a",
		PolicyID:    policy.ID,
		TTLSeconds:  600,
	})
	if err != nil {
		t.Fatalf("create break-glass request failed: %v", err)
	}
	if req.Status != BreakGlassPending || req.CurrentStage != 0 {
		t.Fatalf("unexpected new break-glass request: %+v", req)
	}

	req, err = store.ApproveBreakGlassRequest(req.ID, "reviewer-a", "looks valid")
	if err != nil {
		t.Fatalf("stage-1 approve failed: %v", err)
	}
	if req.Status != BreakGlassPending || req.CurrentStage != 1 {
		t.Fatalf("expected stage advance to stage 2, got %+v", req)
	}

	req, err = store.ApproveBreakGlassRequest(req.ID, "security-a", "approved")
	if err != nil {
		t.Fatalf("stage-2 first approve failed: %v", err)
	}
	if req.Status != BreakGlassPending || req.CurrentStage != 1 {
		t.Fatalf("expected stage-2 to remain pending after first approval, got %+v", req)
	}

	req, err = store.ApproveBreakGlassRequest(req.ID, "security-b", "approved")
	if err != nil {
		t.Fatalf("stage-2 second approve failed: %v", err)
	}
	if req.Status != BreakGlassActive || req.ActivatedAt == nil || req.ExpiresAt == nil {
		t.Fatalf("expected break-glass activation, got %+v", req)
	}
}

func TestBreakGlassRejectRevokeAndExpiry(t *testing.T) {
	store := NewAccessApprovalStore()
	policy, err := store.CreatePolicy(QuorumApprovalPolicyInput{
		Name: "single-stage",
		Stages: []ApprovalStageRule{
			{Name: "approval", RequiredApprovals: 1},
		},
	})
	if err != nil {
		t.Fatalf("create policy failed: %v", err)
	}

	rejectReq, err := store.CreateBreakGlassRequest(BreakGlassRequestInput{
		RequestedBy: "sre",
		Reason:      "db emergency",
		Scope:       "db/prod",
		PolicyID:    policy.ID,
		TTLSeconds:  600,
	})
	if err != nil {
		t.Fatalf("create reject request failed: %v", err)
	}
	rejectReq, err = store.RejectBreakGlassRequest(rejectReq.ID, "manager", "insufficient context")
	if err != nil {
		t.Fatalf("reject request failed: %v", err)
	}
	if rejectReq.Status != BreakGlassRejected || rejectReq.RejectedAt == nil {
		t.Fatalf("expected rejected request, got %+v", rejectReq)
	}

	revokeReq, err := store.CreateBreakGlassRequest(BreakGlassRequestInput{
		RequestedBy: "sre2",
		Reason:      "edge recovery",
		Scope:       "edge/prod",
		PolicyID:    policy.ID,
		TTLSeconds:  600,
	})
	if err != nil {
		t.Fatalf("create revoke request failed: %v", err)
	}
	revokeReq, err = store.RevokeBreakGlassRequest(revokeReq.ID, "manager", "no longer needed")
	if err != nil {
		t.Fatalf("revoke request failed: %v", err)
	}
	if revokeReq.Status != BreakGlassRevoked || revokeReq.RevokedAt == nil {
		t.Fatalf("expected revoked request, got %+v", revokeReq)
	}

	expireReq, err := store.CreateBreakGlassRequest(BreakGlassRequestInput{
		RequestedBy: "sre3",
		Reason:      "cache outage",
		Scope:       "cache/prod",
		PolicyID:    policy.ID,
		TTLSeconds:  600,
	})
	if err != nil {
		t.Fatalf("create expire request failed: %v", err)
	}
	expireReq, err = store.ApproveBreakGlassRequest(expireReq.ID, "approver", "go")
	if err != nil {
		t.Fatalf("activate request failed: %v", err)
	}
	if expireReq.Status != BreakGlassActive {
		t.Fatalf("expected active request before expiry simulation, got %+v", expireReq)
	}

	store.mu.Lock()
	record := store.requests[expireReq.ID]
	expiredAt := time.Now().UTC().Add(-time.Second)
	record.ExpiresAt = &expiredAt
	store.expireBreakGlassRequestsLocked(time.Now().UTC())
	store.mu.Unlock()

	got, ok := store.GetBreakGlassRequest(expireReq.ID)
	if !ok {
		t.Fatalf("expected request to exist")
	}
	if got.Status != BreakGlassExpired {
		t.Fatalf("expected expired status, got %+v", got)
	}
}
