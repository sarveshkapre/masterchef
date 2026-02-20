package control

import "testing"

func TestEvaluateScaleProfile(t *testing.T) {
	small := EvaluateScaleProfile(ScaleProfileEvaluateInput{
		NodeCount:   80,
		TenantCount: 3,
		RegionCount: 1,
		QueueDepth:  40,
	})
	if small.Profile.ID != "scale-small" {
		t.Fatalf("expected small profile, got %+v", small.Profile)
	}
	if small.RecommendedWorkers < 1 {
		t.Fatalf("expected workers recommendation, got %+v", small)
	}

	large := EvaluateScaleProfile(ScaleProfileEvaluateInput{
		NodeCount:   12000,
		TenantCount: 80,
		RegionCount: 4,
		QueueDepth:  6000,
	})
	if large.Profile.ID != "scale-large" {
		t.Fatalf("expected large profile, got %+v", large.Profile)
	}
	if large.RecommendedShards <= 1 {
		t.Fatalf("expected sharded recommendation for large fleet, got %+v", large)
	}
	if len(large.Risks) == 0 {
		t.Fatalf("expected large profile risks for queue and topology pressure, got %+v", large)
	}
}
