package control

import "testing"

func TestPerformanceGateEvaluatePassAndFail(t *testing.T) {
	store := NewPerformanceGateStore()
	if _, err := store.SetPolicy(PerformanceGatePolicy{
		MaxP95LatencyMS:    1000,
		MinThroughputRPS:   200,
		MaxErrorBudgetBurn: 1.2,
		MinSampleCount:     100,
	}); err != nil {
		t.Fatalf("set performance policy failed: %v", err)
	}

	pass, err := store.Evaluate(PerformanceGateSample{
		Component:           "scheduler",
		P95LatencyMS:        800,
		ThroughputRPS:       250,
		ErrorBudgetBurnRate: 0.7,
		SampleCount:         120,
	})
	if err != nil {
		t.Fatalf("evaluate pass sample failed: %v", err)
	}
	if !pass.Pass || len(pass.BlockReasons) != 0 {
		t.Fatalf("expected passing evaluation, got %+v", pass)
	}

	fail, err := store.Evaluate(PerformanceGateSample{
		Component:           "scheduler",
		P95LatencyMS:        1500,
		ThroughputRPS:       90,
		ErrorBudgetBurnRate: 1.8,
		SampleCount:         70,
	})
	if err != nil {
		t.Fatalf("evaluate failing sample failed: %v", err)
	}
	if fail.Pass || len(fail.BlockReasons) < 2 {
		t.Fatalf("expected failing evaluation with reasons, got %+v", fail)
	}
}
