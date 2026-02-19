package control

import "testing"

func TestRebootOrchestrationStorePolicyAndPlan(t *testing.T) {
	store := NewRebootOrchestrationStore()
	policy, err := store.UpsertPolicy(RebootPolicyInput{
		Environment:          "prod",
		MaxConcurrentReboots: 2,
		MinHealthyPercent:    75,
		DependencyOrder:      []string{"db", "api"},
	})
	if err != nil {
		t.Fatalf("upsert reboot policy failed: %v", err)
	}
	if policy.ID == "" {
		t.Fatalf("expected reboot policy id")
	}

	plan := store.Plan(RebootPlanInput{
		Environment: "prod",
		Hosts: []RebootHost{
			{ID: "db-1", Role: "db", FailureDomain: "rack-a", Healthy: true},
			{ID: "db-2", Role: "db", FailureDomain: "rack-b", Healthy: true},
			{ID: "api-1", Role: "api", FailureDomain: "rack-a", Healthy: true},
			{ID: "api-2", Role: "api", FailureDomain: "rack-b", Healthy: true},
		},
	})
	if !plan.Allowed || len(plan.Waves) != 2 || plan.Waves[0].Role != "db" {
		t.Fatalf("unexpected reboot plan %+v", plan)
	}
}

func TestRebootOrchestrationStoreBlocksLowHealth(t *testing.T) {
	store := NewRebootOrchestrationStore()
	_, _ = store.UpsertPolicy(RebootPolicyInput{
		Environment:          "prod",
		MaxConcurrentReboots: 1,
		MinHealthyPercent:    90,
	})
	plan := store.Plan(RebootPlanInput{
		Environment: "prod",
		Hosts: []RebootHost{
			{ID: "api-1", Role: "api", Healthy: true},
			{ID: "api-2", Role: "api", Healthy: false},
		},
	})
	if plan.Allowed {
		t.Fatalf("expected low-health reboot plan block, got %+v", plan)
	}
}
