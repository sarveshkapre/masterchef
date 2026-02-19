package control

import "testing"

func TestUpgradeOrchestrationStoreCreateAdvanceAbort(t *testing.T) {
	store := NewUpgradeOrchestrationStore()
	plan, err := store.CreatePlan(UpgradeOrchestrationPlanInput{
		Component:      "agent",
		FromChannel:    "stable",
		ToChannel:      "candidate",
		Strategy:       "wave",
		TotalNodes:     5,
		WaveSize:       2,
		MaxUnavailable: 1,
	})
	if err != nil {
		t.Fatalf("create upgrade plan failed: %v", err)
	}
	if plan.ID == "" {
		t.Fatalf("expected plan id")
	}

	advanced, err := store.Advance(plan.ID, UpgradeOrchestrationAdvanceInput{Healthy: true})
	if err != nil {
		t.Fatalf("advance wave failed: %v", err)
	}
	if advanced.UpgradedNodes != 2 || advanced.CurrentWave != 1 {
		t.Fatalf("unexpected advanced plan: %+v", advanced)
	}

	blocked, err := store.Advance(plan.ID, UpgradeOrchestrationAdvanceInput{Healthy: false})
	if err != nil {
		t.Fatalf("blocking update should not return error: %v", err)
	}
	if blocked.Status != "blocked" {
		t.Fatalf("expected blocked status, got %+v", blocked)
	}

	aborted, err := store.Abort(plan.ID, UpgradeOrchestrationAbortInput{Reason: "maintenance window ended"})
	if err != nil {
		t.Fatalf("abort plan failed: %v", err)
	}
	if aborted.Status != "aborted" {
		t.Fatalf("expected aborted plan, got %+v", aborted)
	}
}

func TestUpgradeOrchestrationStoreValidation(t *testing.T) {
	store := NewUpgradeOrchestrationStore()
	if _, err := store.CreatePlan(UpgradeOrchestrationPlanInput{
		Component:   "agent",
		FromChannel: "stable",
		ToChannel:   "candidate",
		TotalNodes:  0,
	}); err == nil {
		t.Fatalf("expected total_nodes validation error")
	}
	if _, err := store.Advance("missing", UpgradeOrchestrationAdvanceInput{Healthy: true}); err == nil {
		t.Fatalf("expected missing plan advance error")
	}
}
