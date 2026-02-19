package control

import "testing"

func TestPropertyHarnessCaseAndRun(t *testing.T) {
	store := NewPropertyHarnessStore()
	item, err := store.UpsertCase(PropertyHarnessCase{
		ID:               "property-service-restart",
		Name:             "Service Restart Invariants",
		Provider:         "service",
		ResourceType:     "service",
		Invariants:       []string{"idempotent_restart", "converges_to_running"},
		GeneratedSamples: 90,
	})
	if err != nil {
		t.Fatalf("upsert property harness case failed: %v", err)
	}
	if item.ID != "property-service-restart" {
		t.Fatalf("unexpected property harness case: %+v", item)
	}
	run, err := store.Run(PropertyHarnessRunInput{
		CaseID:      item.ID,
		Seed:        42,
		TriggeredBy: "ci",
	})
	if err != nil {
		t.Fatalf("run property harness failed: %v", err)
	}
	if run.ID == "" || run.CaseID != item.ID || run.Status == "" {
		t.Fatalf("unexpected property harness run: %+v", run)
	}
}
