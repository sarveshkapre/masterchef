package control

import "testing"

func TestPerformanceDiagnosticsStoreSessionsAndDiagnose(t *testing.T) {
	store := NewPerformanceDiagnosticsStore()
	session, err := store.StartSession(ProfileSessionInput{
		Component:   "scheduler",
		Environment: "prod",
		DurationSec: 600,
		SamplingHz:  20,
	})
	if err != nil {
		t.Fatalf("start profile session failed: %v", err)
	}
	if session.ID == "" {
		t.Fatalf("expected session id")
	}
	if len(store.ListSessions()) != 1 {
		t.Fatalf("expected one session")
	}

	diag, err := store.Diagnose(BottleneckDiagnosticsInput{
		Component:         "scheduler",
		Environment:       "prod",
		QueueDepth:        600,
		WorkerUtilization: 94,
		P95LatencyMs:      220000,
		ErrorRatePercent:  3,
		CPUPercent:        89,
		MemoryPressure:    90,
	})
	if err != nil {
		t.Fatalf("diagnose failed: %v", err)
	}
	if diag.Severity != "high" || diag.PrimaryBottleneck == "healthy" {
		t.Fatalf("expected severe bottleneck diagnostics, got %+v", diag)
	}
	if len(diag.Recommendations) == 0 {
		t.Fatalf("expected recommendations")
	}
}

func TestPerformanceDiagnosticsStoreValidation(t *testing.T) {
	store := NewPerformanceDiagnosticsStore()
	if _, err := store.StartSession(ProfileSessionInput{
		Component:   "scheduler",
		Environment: "",
	}); err == nil {
		t.Fatalf("expected start session validation error")
	}
	if _, err := store.Diagnose(BottleneckDiagnosticsInput{
		Component:    "scheduler",
		Environment:  "prod",
		QueueDepth:   -1,
		P95LatencyMs: 1000,
	}); err == nil {
		t.Fatalf("expected diagnose validation error for negative queue depth")
	}
}
