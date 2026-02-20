package control

import "testing"

func TestAdHocCommandStoreEvaluateGuardrails(t *testing.T) {
	store := NewAdHocCommandStore(100)
	req, allowed, reasons, err := store.Evaluate(AdHocCommandRequest{
		Command:     "rm -rf /tmp/demo",
		RequestedBy: "sre-oncall",
		Reason:      "cleanup",
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}
	if req.Host != "localhost" {
		t.Fatalf("expected default host localhost, got %q", req.Host)
	}
	if allowed || len(reasons) == 0 {
		t.Fatalf("expected blocked command by guardrails, got allowed=%t reasons=%v", allowed, reasons)
	}

	_, allowed, reasons, err = store.Evaluate(AdHocCommandRequest{
		Command:     "echo hello",
		RequestedBy: "sre-oncall",
		Reason:      "diagnostic",
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("evaluate allowed command failed: %v", err)
	}
	if !allowed || len(reasons) != 0 {
		t.Fatalf("expected allowed command, got allowed=%t reasons=%v", allowed, reasons)
	}
}

func TestAdHocCommandStoreSetPolicyAndRecordHistory(t *testing.T) {
	store := NewAdHocCommandStore(2)
	policy, err := store.SetPolicy(AdHocGuardrailPolicy{
		BlockedPatterns:   []string{"passwd"},
		RequireReason:     true,
		MaxTimeoutSeconds: 15,
		AllowExecution:    false,
	})
	if err != nil {
		t.Fatalf("set policy failed: %v", err)
	}
	if policy.MaxTimeoutSeconds != 15 || policy.AllowExecution {
		t.Fatalf("unexpected policy: %+v", policy)
	}

	recorded := store.Record(AdHocCommandResult{
		Command: "echo one",
		Status:  "approved",
		Allowed: true,
		DryRun:  true,
	})
	if recorded.ID == "" {
		t.Fatalf("expected recorded command id")
	}
	store.Record(AdHocCommandResult{Command: "echo two", Status: "approved", Allowed: true, DryRun: true})
	store.Record(AdHocCommandResult{Command: "echo three", Status: "approved", Allowed: true, DryRun: true})

	items := store.List(10)
	if len(items) != 2 {
		t.Fatalf("expected history to enforce limit 2, got %d", len(items))
	}
	if items[0].Command != "echo three" || items[1].Command != "echo two" {
		t.Fatalf("unexpected history ordering/content: %+v", items)
	}
}
