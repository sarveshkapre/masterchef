package control

import "testing"

func TestDriftSLOStorePolicyAndEvaluate(t *testing.T) {
	store := NewDriftSLOStore(10)
	policy, err := store.SetPolicy(DriftSLOPolicy{
		TargetPercent:      99.5,
		WindowHours:        12,
		MinSamples:         10,
		AutoCreateIncident: true,
		IncidentHook:       "event://incident.create",
	})
	if err != nil {
		t.Fatalf("set policy failed: %v", err)
	}
	if policy.TargetPercent != 99.5 || policy.WindowHours != 12 {
		t.Fatalf("unexpected policy: %+v", policy)
	}

	eval := store.Evaluate(DriftSLOEvaluationInput{
		Samples:    20,
		Changed:    0,
		FailedRuns: 0,
	})
	if eval.Status != "healthy" || eval.Breached {
		t.Fatalf("expected healthy eval, got %+v", eval)
	}

	eval = store.Evaluate(DriftSLOEvaluationInput{
		Samples: 20,
		Changed: 5,
	})
	if eval.Status != "breached" || !eval.Breached || !eval.IncidentRecommended {
		t.Fatalf("expected breached eval with incident recommendation, got %+v", eval)
	}

	list := store.List(10)
	if len(list) != 2 {
		t.Fatalf("expected two evaluations in history, got %d", len(list))
	}
	if list[0].ID != eval.ID {
		t.Fatalf("expected newest-first list ordering")
	}
}
