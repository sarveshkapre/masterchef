package control

import "testing"

func TestMutationStorePolicyAndRun(t *testing.T) {
	store := NewMutationStore()
	if _, err := store.SetPolicy(MutationPolicy{
		MinKillRate:       0.7,
		MinMutantsCovered: 80,
	}); err != nil {
		t.Fatalf("set mutation policy failed: %v", err)
	}
	run, err := store.Run(MutationRunInput{
		SuiteID:     "mutation-file-provider",
		Seed:        42,
		TriggeredBy: "ci",
	})
	if err != nil {
		t.Fatalf("run mutation suite failed: %v", err)
	}
	if run.ID == "" || run.Provider != "file" || run.Status == "" {
		t.Fatalf("unexpected mutation run payload: %+v", run)
	}
}

func TestMutationStoreUpsertSuite(t *testing.T) {
	store := NewMutationStore()
	suite, err := store.UpsertSuite(MutationSuite{
		ID:            "mutation-service-provider",
		Provider:      "service",
		Name:          "Service Provider Mutation Tests",
		CriticalPaths: []string{"service/start-stop", "service/restart"},
	})
	if err != nil {
		t.Fatalf("upsert mutation suite failed: %v", err)
	}
	if suite.ID != "mutation-service-provider" || len(suite.CriticalPaths) == 0 {
		t.Fatalf("unexpected mutation suite payload: %+v", suite)
	}
}
