package control

import "testing"

func TestWorkerAutoscalingRecommend(t *testing.T) {
	store := NewWorkerAutoscalingStore()
	_, err := store.SetPolicy(WorkerAutoscalingPolicy{
		Enabled:             true,
		MinWorkers:          2,
		MaxWorkers:          50,
		QueueDepthPerWorker: 10,
		TargetP95LatencyMs:  1000,
		ScaleUpStep:         5,
		ScaleDownStep:       2,
	})
	if err != nil {
		t.Fatalf("set policy failed: %v", err)
	}
	up := store.Recommend(WorkerAutoscalingInput{QueueDepth: 200, CurrentWorkers: 10, P95LatencyMs: 2000})
	if up.Recommended <= 10 {
		t.Fatalf("expected scale up decision, got %+v", up)
	}
	down := store.Recommend(WorkerAutoscalingInput{QueueDepth: 5, CurrentWorkers: 20, P95LatencyMs: 200})
	if down.Recommended >= 20 {
		t.Fatalf("expected scale down decision, got %+v", down)
	}
}
