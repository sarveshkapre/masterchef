package control

import "testing"

func TestEvaluateInvariants(t *testing.T) {
	report := EvaluateInvariants([]Invariant{
		{Name: "error-budget", Field: "error_rate", Comparator: "lte", Value: 0.02, Severity: "critical"},
		{Name: "latency", Field: "p95_ms", Comparator: "lte", Value: 500, Severity: "warning"},
	}, map[string]float64{
		"error_rate": 0.01,
		"p95_ms":     700,
	})
	if !report.Pass {
		t.Fatalf("expected report pass since critical invariant passed: %+v", report)
	}
	if report.FailedCount != 1 {
		t.Fatalf("expected one failed invariant, got %d", report.FailedCount)
	}
	if report.CriticalFail != 0 {
		t.Fatalf("expected zero critical failures")
	}
}
