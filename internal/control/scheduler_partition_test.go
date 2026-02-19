package control

import "testing"

func TestSchedulerPartitionStoreRulesAndDecide(t *testing.T) {
	store := NewSchedulerPartitionStore()
	rule, err := store.Upsert(SchedulerPartitionRuleInput{
		Tenant:      "payments",
		Environment: "prod",
		Region:      "us-east-1",
		Shard:       "shard-a",
		MaxParallel: 40,
	})
	if err != nil {
		t.Fatalf("upsert partition rule failed: %v", err)
	}
	if rule.ID == "" || rule.Shard != "shard-a" {
		t.Fatalf("unexpected rule %+v", rule)
	}
	decision := store.Decide(SchedulerPartitionDecisionInput{
		Tenant:      "payments",
		Environment: "prod",
		Region:      "us-east-1",
		WorkloadKey: "deploy:payments:v1",
	})
	if decision.RuleID == "" || decision.Shard != "shard-a" {
		t.Fatalf("expected rule-backed decision, got %+v", decision)
	}
	fallback := store.Decide(SchedulerPartitionDecisionInput{Tenant: "unknown", WorkloadKey: "x"})
	if fallback.Shard == "" || fallback.Reason == "" {
		t.Fatalf("expected fallback decision, got %+v", fallback)
	}
}
