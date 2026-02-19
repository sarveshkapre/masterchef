package control

import "testing"

func TestDependencyUpdateStoreFlow(t *testing.T) {
	store := NewDependencyUpdateStore()
	item, err := store.Propose(DependencyUpdateInput{
		Ecosystem:      "go",
		Package:        "github.com/example/pkg",
		CurrentVersion: "v1.0.0",
		TargetVersion:  "v1.1.0",
		Reason:         "security fix",
	})
	if err != nil {
		t.Fatalf("propose dependency update failed: %v", err)
	}
	if item.ReadyForMerge {
		t.Fatalf("expected proposal to require verification initially")
	}
	evaluated, err := store.Evaluate(DependencyUpdateEvaluationInput{
		UpdateID:             item.ID,
		CompatibilityChecked: true,
		CompatibilityPassed:  true,
		PerformanceChecked:   true,
		PerformanceDeltaPct:  1.2,
	})
	if err != nil {
		t.Fatalf("evaluate dependency update failed: %v", err)
	}
	if !evaluated.ReadyForMerge {
		t.Fatalf("expected proposal to become merge-ready: %+v", evaluated)
	}
}

func TestDependencyUpdateStorePolicyGuards(t *testing.T) {
	store := NewDependencyUpdateStore()
	if _, err := store.SetPolicy(DependencyUpdatePolicy{
		Enabled:                   true,
		MaxUpdatesPerDay:          1,
		RequireCompatibilityCheck: true,
		RequirePerformanceCheck:   true,
		AllowedEcosystems:         []string{"go"},
	}); err != nil {
		t.Fatalf("set policy failed: %v", err)
	}
	if _, err := store.Propose(DependencyUpdateInput{
		Ecosystem:      "npm",
		Package:        "react",
		CurrentVersion: "18.0.0",
		TargetVersion:  "18.1.0",
	}); err == nil {
		t.Fatalf("expected disallowed ecosystem to fail")
	}
}
