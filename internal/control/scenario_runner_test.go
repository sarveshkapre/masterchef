package control

import "testing"

func TestScenarioStoreRunDeterministic(t *testing.T) {
	store := NewScenarioTestStore()
	runA, err := store.Run(ScenarioRunInput{
		ScenarioID:  "release-canary-fleet",
		Seed:        42,
		TriggeredBy: "ci",
	})
	if err != nil {
		t.Fatalf("run scenario failed: %v", err)
	}
	runB, err := store.Run(ScenarioRunInput{
		ScenarioID: "release-canary-fleet",
		Seed:       42,
	})
	if err != nil {
		t.Fatalf("second run scenario failed: %v", err)
	}
	if runA.NodesFailed != runB.NodesFailed || runA.DriftFindings != runB.DriftFindings || runA.Status != runB.Status {
		t.Fatalf("expected deterministic outputs for same seed: runA=%+v runB=%+v", runA, runB)
	}
}

func TestScenarioStoreUpsertAndLookup(t *testing.T) {
	store := NewScenarioTestStore()
	updated, err := store.UpsertScenario(ScenarioDefinition{
		ID:          "custom-fleet",
		Name:        "Custom Fleet",
		Description: "custom scenario",
		FleetSize:   250,
		Services:    8,
		FailureRate: 0.01,
		ChaosLevel:  10,
	})
	if err != nil {
		t.Fatalf("upsert scenario failed: %v", err)
	}
	if updated.ID != "custom-fleet" || updated.Name != "Custom Fleet" {
		t.Fatalf("unexpected upserted scenario: %+v", updated)
	}
	run, err := store.Run(ScenarioRunInput{ScenarioID: "custom-fleet", Seed: 7})
	if err != nil {
		t.Fatalf("run custom scenario failed: %v", err)
	}
	got, err := store.GetRun(run.ID)
	if err != nil {
		t.Fatalf("get run failed: %v", err)
	}
	if got.ID != run.ID {
		t.Fatalf("unexpected run returned: %+v", got)
	}
}
