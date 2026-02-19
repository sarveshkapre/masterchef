package control

import "testing"

func TestFlakeQuarantineObserveAndAutoQuarantine(t *testing.T) {
	store := NewFlakeQuarantineStore()
	if _, err := store.SetPolicy(FlakePolicy{
		AutoQuarantine:              true,
		MinSamples:                  3,
		FlakeRateThreshold:          0.50,
		ConsecutiveFailureThreshold: 2,
	}); err != nil {
		t.Fatalf("set policy failed: %v", err)
	}

	sequence := []string{"fail", "pass", "fail"}
	var result FlakeObservationResult
	var err error
	for _, status := range sequence {
		result, err = store.Observe(FlakeObservation{
			Suite:  "e2e",
			Test:   "TestCanaryRollout",
			Status: status,
		})
		if err != nil {
			t.Fatalf("observe failed: %v", err)
		}
	}
	if result.Action != "auto-quarantined" || !result.Case.Quarantined {
		t.Fatalf("expected auto quarantine after threshold, got %+v", result)
	}
	summary := store.Summary()
	if summary.Quarantined != 1 || summary.AutoQuarantined != 1 {
		t.Fatalf("unexpected summary %+v", summary)
	}
}

func TestFlakeQuarantineManualOverrides(t *testing.T) {
	store := NewFlakeQuarantineStore()
	result, err := store.Observe(FlakeObservation{
		Suite:  "unit",
		Test:   "TestRetryBackoff",
		Status: "fail",
	})
	if err != nil {
		t.Fatalf("observe failed: %v", err)
	}
	item, err := store.Quarantine(result.Case.ID, "known flaky test")
	if err != nil {
		t.Fatalf("manual quarantine failed: %v", err)
	}
	if !item.Quarantined {
		t.Fatalf("expected quarantined test case")
	}
	item, err = store.Unquarantine(result.Case.ID, "stabilized")
	if err != nil {
		t.Fatalf("manual unquarantine failed: %v", err)
	}
	if item.Quarantined {
		t.Fatalf("expected unquarantined test case")
	}
}
