package control

import "testing"

func TestHealthProbeStoreLifecycleAndGate(t *testing.T) {
	store := NewHealthProbeStore()
	targetA, err := store.UpsertTarget(HealthProbeTargetInput{
		Name:    "api-probe",
		Service: "payments-api",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("upsert target A failed: %v", err)
	}
	targetB, err := store.UpsertTarget(HealthProbeTargetInput{
		Name:    "worker-probe",
		Service: "payments-worker",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("upsert target B failed: %v", err)
	}
	if _, err := store.RecordCheck(HealthProbeCheckInput{TargetID: targetA.ID, Status: "healthy", LatencyMS: 12}); err != nil {
		t.Fatalf("record check A failed: %v", err)
	}
	if _, err := store.RecordCheck(HealthProbeCheckInput{TargetID: targetB.ID, Status: "unhealthy", LatencyMS: 300}); err != nil {
		t.Fatalf("record check B failed: %v", err)
	}
	gate := store.EvaluateGate(HealthProbeGateRequest{
		MinHealthyPercent: 80,
		RecommendRollback: true,
	})
	if gate.Decision != "block" || gate.RecommendedAction != "rollback" {
		t.Fatalf("expected blocked rollback recommendation, got %+v", gate)
	}
}

func TestHealthProbeStoreValidation(t *testing.T) {
	store := NewHealthProbeStore()
	if _, err := store.UpsertTarget(HealthProbeTargetInput{}); err == nil {
		t.Fatalf("expected target validation error")
	}
	target, err := store.UpsertTarget(HealthProbeTargetInput{Name: "ok", Enabled: true})
	if err != nil {
		t.Fatalf("upsert target failed: %v", err)
	}
	if _, err := store.RecordCheck(HealthProbeCheckInput{TargetID: target.ID, Status: "bad"}); err == nil {
		t.Fatalf("expected status validation error")
	}
}
