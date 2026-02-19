package control

import "testing"

func TestTopologyPlacementStorePolicyAndDecision(t *testing.T) {
	store := NewTopologyPlacementStore()
	policy, err := store.Upsert(TopologyPlacementPolicyInput{
		Environment:   "prod",
		Region:        "us-east-1",
		Zone:          "us-east-1a",
		Cluster:       "payments",
		FailureDomain: "rack-a",
		MaxParallel:   15,
	})
	if err != nil {
		t.Fatalf("upsert topology placement policy failed: %v", err)
	}
	if policy.ID == "" {
		t.Fatalf("expected policy id")
	}

	decision := store.Decide(TopologyPlacementDecisionInput{
		Environment:   "prod",
		Region:        "us-east-1",
		Zone:          "us-east-1a",
		Cluster:       "payments",
		FailureDomain: "rack-a",
		RunKey:        "deploy:v1",
	})
	if decision.PolicyID == "" || decision.MaxParallel != 15 {
		t.Fatalf("expected policy-backed decision, got %+v", decision)
	}

	fallback := store.Decide(TopologyPlacementDecisionInput{
		Environment: "dev",
		Region:      "us-west-2",
	})
	if fallback.PolicyID != "" || fallback.MaxParallel != 10 {
		t.Fatalf("expected default fallback decision, got %+v", fallback)
	}
}

func TestTopologyPlacementStoreValidation(t *testing.T) {
	store := NewTopologyPlacementStore()
	if _, err := store.Upsert(TopologyPlacementPolicyInput{
		Environment: "",
	}); err == nil {
		t.Fatalf("expected missing environment validation error")
	}
}
