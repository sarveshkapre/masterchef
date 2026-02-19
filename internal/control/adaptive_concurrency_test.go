package control

import "testing"

func TestAdaptiveConcurrencyRecommendScaleDownOnFailures(t *testing.T) {
	store := NewAdaptiveConcurrencyStore()
	decision := store.Recommend(AdaptiveConcurrencyInput{
		CurrentParallelism: 20,
		RecentFailureRate:  0.35,
		HostHealth:         map[string]string{"web-1": "healthy"},
	})
	if decision.RecommendedParallelism != store.Policy().MinParallelism {
		t.Fatalf("expected min parallelism under critical failures, got %+v", decision)
	}
}

func TestAdaptiveConcurrencyRecommendScaleUpOnHealthyBacklog(t *testing.T) {
	store := NewAdaptiveConcurrencyStore()
	decision := store.Recommend(AdaptiveConcurrencyInput{
		CurrentParallelism: 10,
		RecentFailureRate:  0.02,
		HostHealth:         map[string]string{"web-1": "healthy", "web-2": "healthy"},
		Backlog:            300,
	})
	if decision.RecommendedParallelism <= 10 {
		t.Fatalf("expected scale up under healthy backlog, got %+v", decision)
	}
}
