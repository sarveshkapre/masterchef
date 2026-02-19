package control

import "testing"

func TestChaosExperimentLifecycle(t *testing.T) {
	store := NewChaosExperimentStore()
	async, err := store.Create(ChaosExperimentInput{
		Name:        "queue-latency-chaos",
		Target:      "staging/queue",
		FaultType:   "queue-delay",
		Intensity:   45,
		DurationSec: 120,
		Async:       true,
		TriggeredBy: "sre",
	})
	if err != nil {
		t.Fatalf("create async chaos experiment failed: %v", err)
	}
	if async.Status != "running" {
		t.Fatalf("expected running status for async experiment, got %+v", async)
	}

	completed, err := store.Complete(async.ID)
	if err != nil {
		t.Fatalf("complete async experiment failed: %v", err)
	}
	if completed.Status != "completed" || completed.ImpactScore == 0 {
		t.Fatalf("expected completed experiment with impact score, got %+v", completed)
	}

	syncExp, err := store.Create(ChaosExperimentInput{
		Name:        "process-crash-chaos",
		Target:      "staging/orchestrator",
		FaultType:   "process-crash",
		Intensity:   30,
		DurationSec: 60,
		Async:       false,
	})
	if err != nil {
		t.Fatalf("create sync chaos experiment failed: %v", err)
	}
	if syncExp.Status != "completed" {
		t.Fatalf("expected completed sync experiment, got %+v", syncExp)
	}
}

func TestChaosExperimentProductionGuardrail(t *testing.T) {
	store := NewChaosExperimentStore()
	blocked, err := store.Create(ChaosExperimentInput{
		Name:        "prod-network-chaos",
		Target:      "prod/payments",
		FaultType:   "network-latency",
		Intensity:   90,
		DurationSec: 180,
	})
	if err != nil {
		t.Fatalf("create blocked chaos experiment failed: %v", err)
	}
	if blocked.Status != "blocked" || len(blocked.Findings) == 0 {
		t.Fatalf("expected blocked chaos experiment with finding, got %+v", blocked)
	}
}
