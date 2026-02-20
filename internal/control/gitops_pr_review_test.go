package control

import "testing"

func TestGitOpsPRReviewStore(t *testing.T) {
	store := NewGitOpsPRReviewStore()

	gate, err := store.UpsertGate(GitOpsApprovalGate{
		Repository:        "github.com/masterchef/masterchef",
		Environment:       "prod",
		MinApprovals:      2,
		RequiredChecks:    []string{"plan/simulate", "plan/reproducibility"},
		RequiredReviewers: []string{"platform-oncall"},
		BlockRiskLevels:   []string{"high", "critical"},
	})
	if err != nil {
		t.Fatalf("upsert gate failed: %v", err)
	}
	if gate.ID == "" || gate.MinApprovals != 2 {
		t.Fatalf("unexpected gate: %+v", gate)
	}

	comment, err := store.AddComment(GitOpsPRCommentInput{
		Repository:       "github.com/masterchef/masterchef",
		PRNumber:         101,
		Environment:      "prod",
		PlanSummary:      "Plan touches 24 hosts and 3 services.",
		RiskLevel:        "high",
		SuggestedActions: []string{"require two approvers", "run canary first"},
	})
	if err != nil {
		t.Fatalf("add comment failed: %v", err)
	}
	if comment.ID == "" || comment.RiskLevel != "high" {
		t.Fatalf("unexpected comment: %+v", comment)
	}

	comments := store.ListComments("github.com/masterchef/masterchef", 101, 10)
	if len(comments) != 1 {
		t.Fatalf("expected one comment, got %d", len(comments))
	}

	blocked, err := store.Evaluate(GitOpsApprovalEvaluationInput{
		Repository:    "github.com/masterchef/masterchef",
		Environment:   "prod",
		PRNumber:      101,
		RiskLevel:     "high",
		ApprovalCount: 2,
		PassedChecks:  []string{"plan/simulate"},
		Reviewers:     []string{"platform-oncall"},
	})
	if err != nil {
		t.Fatalf("evaluate blocked failed: %v", err)
	}
	if blocked.Allowed {
		t.Fatalf("expected blocked approval decision, got %+v", blocked)
	}

	allowed, err := store.Evaluate(GitOpsApprovalEvaluationInput{
		GateID:        gate.ID,
		Repository:    "github.com/masterchef/masterchef",
		Environment:   "prod",
		PRNumber:      101,
		RiskLevel:     "medium",
		ApprovalCount: 2,
		PassedChecks:  []string{"plan/simulate", "plan/reproducibility"},
		Reviewers:     []string{"platform-oncall"},
	})
	if err != nil {
		t.Fatalf("evaluate allowed failed: %v", err)
	}
	if !allowed.Allowed {
		t.Fatalf("expected allowed decision, got %+v", allowed)
	}
}
