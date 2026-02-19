package control

import "testing"

func TestDisruptionBudgetEvaluation(t *testing.T) {
	store := NewDisruptionBudgetStore()
	b, err := store.Create(DisruptionBudgetInput{
		Name:           "prod-budget",
		MaxUnavailable: 2,
		MinHealthyPct:  80,
	})
	if err != nil {
		t.Fatalf("create budget failed: %v", err)
	}
	okEval := EvaluateDisruptionBudget(b, 10, 2)
	if !okEval.Allowed {
		t.Fatalf("expected budget evaluation to allow request: %+v", okEval)
	}
	blockEval := EvaluateDisruptionBudget(b, 10, 3)
	if blockEval.Allowed {
		t.Fatalf("expected budget evaluation to block request: %+v", blockEval)
	}
}
