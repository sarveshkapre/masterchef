package control

import "testing"

func TestProviderConformanceRunDeterministic(t *testing.T) {
	store := NewProviderConformanceStore()
	runA, err := store.Run(ProviderConformanceRunInput{
		SuiteID:         "provider-file-core",
		ProviderVersion: "v1.2.3",
		Trigger:         "nightly",
	})
	if err != nil {
		t.Fatalf("run conformance failed: %v", err)
	}
	runB, err := store.Run(ProviderConformanceRunInput{
		SuiteID:         "provider-file-core",
		ProviderVersion: "v1.2.3",
		Trigger:         "nightly",
	})
	if err != nil {
		t.Fatalf("second run conformance failed: %v", err)
	}
	if runA.PassRate != runB.PassRate || runA.FailedChecks != runB.FailedChecks || runA.Status != runB.Status {
		t.Fatalf("expected deterministic results for same input: runA=%+v runB=%+v", runA, runB)
	}
}

func TestProviderConformanceSuiteUpsert(t *testing.T) {
	store := NewProviderConformanceStore()
	suite, err := store.UpsertSuite(ProviderConformanceSuite{
		ID:               "provider-registry-core",
		Provider:         "registry",
		Description:      "registry provider checks",
		Checks:           []string{"index-sync", "signature-verify", "metadata-lookup"},
		RequiredPassRate: 0.9,
	})
	if err != nil {
		t.Fatalf("upsert suite failed: %v", err)
	}
	if suite.ID != "provider-registry-core" || suite.Provider != "registry" {
		t.Fatalf("unexpected suite response: %+v", suite)
	}
	run, err := store.Run(ProviderConformanceRunInput{SuiteID: suite.ID, ProviderVersion: "v0.9.0"})
	if err != nil {
		t.Fatalf("run custom suite failed: %v", err)
	}
	if run.ID == "" || run.SuiteID != suite.ID {
		t.Fatalf("unexpected run response: %+v", run)
	}
}
