package control

import "testing"

func TestEvaluateReadinessPassAndFail(t *testing.T) {
	thresholds := DefaultReadinessThresholds()
	pass := EvaluateReadiness(ReadinessSignals{
		QualityScore:          0.9,
		ReliabilityScore:      0.95,
		PerformanceScore:      0.9,
		TestPassRate:          0.99,
		FlakeRate:             0.01,
		OpenCriticalIncidents: 0,
		P95ApplyLatencyMs:     1000,
	}, thresholds)
	if !pass.Pass || len(pass.Blockers) != 0 {
		t.Fatalf("expected readiness pass, got %+v", pass)
	}

	fail := EvaluateReadiness(ReadinessSignals{
		QualityScore:          0.5,
		ReliabilityScore:      0.5,
		PerformanceScore:      0.5,
		TestPassRate:          0.8,
		FlakeRate:             0.2,
		OpenCriticalIncidents: 2,
		P95ApplyLatencyMs:     500000,
	}, thresholds)
	if fail.Pass || len(fail.Blockers) < 3 {
		t.Fatalf("expected readiness fail with blockers, got %+v", fail)
	}
}
