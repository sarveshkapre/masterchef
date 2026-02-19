package control

import "testing"

func TestWorkspaceIsolationStorePolicyAndEvaluate(t *testing.T) {
	store := NewWorkspaceIsolationStore()
	policy, err := store.Upsert(WorkspaceIsolationPolicyInput{
		Tenant:                  "acme",
		Workspace:               "payments",
		Environment:             "prod",
		NetworkSegment:          "seg-prod-payments",
		ComputePool:             "pool-payments",
		DataScope:               "acme/payments",
		AllowCrossWorkspaceRead: false,
	})
	if err != nil {
		t.Fatalf("upsert workspace isolation policy failed: %v", err)
	}
	if policy.ID == "" {
		t.Fatalf("expected policy id")
	}

	allowed := store.Evaluate(WorkspaceIsolationEvaluateInput{
		Tenant:             "acme",
		Workspace:          "payments",
		Environment:        "prod",
		TargetWorkspace:    "payments",
		RequestedDataScope: "acme/payments",
		NetworkSegment:     "seg-prod-payments",
		ComputePool:        "pool-payments",
	})
	if !allowed.Allowed {
		t.Fatalf("expected allowed decision, got %+v", allowed)
	}

	crossWorkspace := store.Evaluate(WorkspaceIsolationEvaluateInput{
		Tenant:          "acme",
		Workspace:       "payments",
		Environment:     "prod",
		TargetWorkspace: "core",
	})
	if crossWorkspace.Allowed {
		t.Fatalf("expected cross-workspace deny, got %+v", crossWorkspace)
	}

	networkMismatch := store.Evaluate(WorkspaceIsolationEvaluateInput{
		Tenant:         "acme",
		Workspace:      "payments",
		Environment:    "prod",
		NetworkSegment: "seg-prod-core",
	})
	if networkMismatch.Allowed {
		t.Fatalf("expected network segment mismatch deny, got %+v", networkMismatch)
	}

	missing := store.Evaluate(WorkspaceIsolationEvaluateInput{
		Tenant:      "acme",
		Workspace:   "orders",
		Environment: "prod",
	})
	if missing.Allowed {
		t.Fatalf("expected deny for missing policy, got %+v", missing)
	}
}

func TestWorkspaceIsolationStoreValidation(t *testing.T) {
	store := NewWorkspaceIsolationStore()
	if _, err := store.Upsert(WorkspaceIsolationPolicyInput{
		Tenant:      "acme",
		Workspace:   "payments",
		Environment: "prod",
	}); err == nil {
		t.Fatalf("expected validation failure for missing segment/pool")
	}
}
