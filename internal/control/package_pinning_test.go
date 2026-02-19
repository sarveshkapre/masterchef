package control

import "testing"

func TestPackagePinStorePolicyAndEvaluate(t *testing.T) {
	store := NewPackagePinStore()
	policy, err := store.Upsert(PackagePinPolicyInput{
		TargetKind:   "host",
		Target:       "node-1",
		Package:      "nginx",
		Version:      "1.25.4",
		Held:         true,
		EnforceDrift: true,
	})
	if err != nil {
		t.Fatalf("upsert package pin policy failed: %v", err)
	}
	if policy.ID == "" {
		t.Fatalf("expected policy id")
	}

	drift := store.Evaluate(PackagePinEvaluateInput{
		TargetKind:       "host",
		Target:           "node-1",
		Package:          "nginx",
		InstalledVersion: "1.26.0",
		HoldApplied:      true,
	})
	if !drift.DriftDetected || drift.Action != "downgrade" {
		t.Fatalf("expected drift downgrade decision, got %+v", drift)
	}

	hold := store.Evaluate(PackagePinEvaluateInput{
		TargetKind:       "host",
		Target:           "node-1",
		Package:          "nginx",
		InstalledVersion: "1.25.4",
		HoldApplied:      false,
	})
	if hold.Action != "hold" {
		t.Fatalf("expected hold action, got %+v", hold)
	}
}

func TestPackagePinStoreValidation(t *testing.T) {
	store := NewPackagePinStore()
	if _, err := store.Upsert(PackagePinPolicyInput{
		TargetKind: "host",
		Target:     "node-1",
		Package:    "nginx",
	}); err == nil {
		t.Fatalf("expected missing version validation error")
	}
}
