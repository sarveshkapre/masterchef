package control

import "testing"

func TestProviderFixtureHarnessStoreRun(t *testing.T) {
	conformance := NewProviderConformanceStore()
	store := NewProviderFixtureHarnessStore(100)

	fixtureA, err := store.UpsertFixture(ProviderTestFixture{
		Provider:       "file",
		Name:           "idempotent-file-apply",
		ExpectedChecks: []string{"idempotency", "drift-detection"},
		Tags:           []string{"core", "idempotency"},
	})
	if err != nil {
		t.Fatalf("upsert fixture A failed: %v", err)
	}
	_, err = store.UpsertFixture(ProviderTestFixture{
		Provider:       "file",
		Name:           "permission-reconcile",
		ExpectedChecks: []string{"permission-reconcile"},
		Tags:           []string{"permissions"},
	})
	if err != nil {
		t.Fatalf("upsert fixture B failed: %v", err)
	}

	items := store.ListFixtures("file", 10)
	if len(items) != 2 {
		t.Fatalf("expected two fixtures, got %d", len(items))
	}

	run, err := store.Run(ProviderHarnessRunInput{
		Provider:   "file",
		SuiteID:    "provider-file-core",
		FixtureIDs: []string{fixtureA.ID},
	}, conformance)
	if err != nil {
		t.Fatalf("run harness failed: %v", err)
	}
	if run.Provider != "file" || run.SuiteID != "provider-file-core" {
		t.Fatalf("unexpected harness run metadata: %+v", run)
	}
	if len(run.FixtureResults) != 1 {
		t.Fatalf("expected one fixture result, got %+v", run)
	}

	listed := store.ListRuns("file", 10)
	if len(listed) != 1 {
		t.Fatalf("expected one listed harness run, got %d", len(listed))
	}
	got, err := store.GetRun(run.ID)
	if err != nil {
		t.Fatalf("get harness run failed: %v", err)
	}
	if got.ID != run.ID {
		t.Fatalf("unexpected harness run lookup: %+v", got)
	}
}
