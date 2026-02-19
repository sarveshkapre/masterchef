package control

import "testing"

func TestRolloutControlStorePoliciesAndPlans(t *testing.T) {
	store := NewRolloutControlStore()
	policy, err := store.UpsertPolicy(RolloutPolicyInput{
		Environment:   "prod",
		Strategy:      "rolling",
		Mode:          "percentage",
		BatchPercent:  50,
		CanaryPercent: 20,
	})
	if err != nil {
		t.Fatalf("upsert rollout policy failed: %v", err)
	}
	if policy.ID == "" {
		t.Fatalf("expected rollout policy id")
	}

	plan := store.Plan(RolloutPlanInput{
		Environment: "prod",
		Targets:     []string{"a", "b", "c", "d"},
	})
	if !plan.Allowed || len(plan.Waves) != 2 {
		t.Fatalf("expected two rollout waves for 50%% batches, got %+v", plan)
	}

	_, err = store.UpsertPolicy(RolloutPolicyInput{
		Environment:   "prod",
		Strategy:      "canary",
		Mode:          "batch",
		CanaryPercent: 25,
	})
	if err != nil {
		t.Fatalf("upsert canary policy failed: %v", err)
	}
	canary := store.Plan(RolloutPlanInput{
		Environment: "prod",
		Targets:     []string{"a", "b", "c", "d"},
	})
	if canary.Strategy != "canary" || len(canary.Waves) < 1 {
		t.Fatalf("expected canary rollout waves, got %+v", canary)
	}
}

func TestRolloutControlStoreValidation(t *testing.T) {
	store := NewRolloutControlStore()
	if _, err := store.UpsertPolicy(RolloutPolicyInput{
		Environment: "prod",
		Strategy:    "invalid",
		Mode:        "serial",
	}); err == nil {
		t.Fatalf("expected invalid strategy error")
	}
}
