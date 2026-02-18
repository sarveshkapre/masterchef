package control

import "testing"

func TestEvaluateReleaseBlocker(t *testing.T) {
	policy := DefaultReleaseBlockerPolicy()
	report := EvaluateReleaseBlocker(ReleaseBlockerInput{
		Readiness: ReadinessReport{
			Pass:           true,
			AggregateScore: 0.96,
		},
		APIDiff: &APIDiffReport{
			DeprecationLifecyclePass: true,
		},
		SimulationConfidence: 0.98,
	}, policy)
	if !report.Pass {
		t.Fatalf("expected release blocker to pass: %+v", report)
	}
	if report.CraftsmanshipTier != "gold" {
		t.Fatalf("expected gold craftsmanship tier, got %s", report.CraftsmanshipTier)
	}
}

func TestEvaluateReleaseBlockerBlocks(t *testing.T) {
	policy := DefaultReleaseBlockerPolicy()
	report := EvaluateReleaseBlocker(ReleaseBlockerInput{
		Readiness: ReadinessReport{
			Pass:           false,
			AggregateScore: 0.70,
		},
		APIDiff: &APIDiffReport{
			DeprecationLifecyclePass: false,
		},
		SimulationConfidence: 0.5,
	}, policy)
	if report.Pass {
		t.Fatalf("expected release blocker to fail")
	}
	if len(report.Blockers) < 3 {
		t.Fatalf("expected multiple blockers, got %+v", report.Blockers)
	}
	if report.CraftsmanshipTier != "bronze" {
		t.Fatalf("expected bronze craftsmanship tier, got %s", report.CraftsmanshipTier)
	}
}
