package control

import (
	"context"
	"testing"
	"time"
)

func TestCanaryStore_TracksHealthAndEnableDisable(t *testing.T) {
	q := NewQueue(32)
	exec := &fakeExecutor{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.StartWorker(ctx, exec)

	cs := NewCanaryStore(q)
	canary, err := cs.Create(CanaryCreate{
		Name:       "control-plane",
		ConfigPath: "ok.yaml",
		Interval:   20 * time.Millisecond,
		Priority:   "high",
	})
	if err != nil {
		t.Fatalf("unexpected canary create error: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		cur, err := cs.Get(canary.ID)
		if err != nil {
			t.Fatalf("unexpected canary get error: %v", err)
		}
		if cur.Health == CanaryHealthy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for healthy canary status")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if _, err := cs.SetEnabled(canary.ID, false); err != nil {
		t.Fatalf("unexpected canary disable error: %v", err)
	}
	cur, err := cs.Get(canary.ID)
	if err != nil {
		t.Fatalf("unexpected canary get after disable error: %v", err)
	}
	if cur.Enabled {
		t.Fatalf("expected canary to be disabled")
	}
}

func TestCanaryStore_UnhealthyAfterFailures(t *testing.T) {
	q := NewQueue(32)
	exec := &fakeExecutor{failOn: "bad.yaml"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.StartWorker(ctx, exec)

	cs := NewCanaryStore(q)
	canary, err := cs.Create(CanaryCreate{
		Name:             "failing",
		ConfigPath:       "bad.yaml",
		Interval:         20 * time.Millisecond,
		FailureThreshold: 1,
	})
	if err != nil {
		t.Fatalf("unexpected canary create error: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		cur, err := cs.Get(canary.ID)
		if err != nil {
			t.Fatalf("unexpected canary get error: %v", err)
		}
		if cur.Health == CanaryUnhealthy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for unhealthy canary status")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
