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

func TestScenarioStoreBaselineAndRegression(t *testing.T) {
	store := NewScenarioTestStore()
	runA, err := store.Run(ScenarioRunInput{ScenarioID: "release-canary-fleet", Seed: 100})
	if err != nil {
		t.Fatalf("run baseline candidate failed: %v", err)
	}
	baseline, err := store.CreateBaseline(ScenarioBaselineInput{
		Name:  "golden-canary",
		RunID: runA.ID,
	})
	if err != nil {
		t.Fatalf("create baseline failed: %v", err)
	}
	runB, err := store.Run(ScenarioRunInput{ScenarioID: "release-canary-fleet", Seed: 101})
	if err != nil {
		t.Fatalf("run comparison candidate failed: %v", err)
	}
	store.mu.Lock()
	store.runs[runB.ID].NodesFailed = runA.NodesFailed + 50
	store.runs[runB.ID].NodesSucceeded = store.runs[runB.ID].NodesTotal - store.runs[runB.ID].NodesFailed
	store.runs[runB.ID].DriftFindings = runA.DriftFindings + 10
	store.runs[runB.ID].MeanApplyLatencyMS = runA.MeanApplyLatencyMS + 200
	store.mu.Unlock()

	report, err := store.CompareRunToBaseline(runB.ID, baseline.ID)
	if err != nil {
		t.Fatalf("compare run to baseline failed: %v", err)
	}
	if !report.RegressionDetected || len(report.Reasons) == 0 {
		t.Fatalf("expected regression detection, got %+v", report)
	}

	withBaseline, err := store.Run(ScenarioRunInput{ScenarioID: "release-canary-fleet", Seed: 102, BaselineID: baseline.ID})
	if err != nil {
		t.Fatalf("run with baseline failed: %v", err)
	}
	if withBaseline.BaselineID != baseline.ID {
		t.Fatalf("expected baseline id to be attached, got %+v", withBaseline)
	}
}
