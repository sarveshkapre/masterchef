package control

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeExecutor struct {
	mu     sync.Mutex
	calls  int
	failOn string
}

func (f *fakeExecutor) ApplyPath(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if path == f.failOn {
		return context.DeadlineExceeded
	}
	return nil
}

func TestQueue_IdempotencyKeyReturnsSameJob(t *testing.T) {
	q := NewQueue(16)
	j1, err := q.Enqueue("a.yaml", "k1", false)
	if err != nil {
		t.Fatalf("unexpected enqueue error: %v", err)
	}
	j2, err := q.Enqueue("b.yaml", "k1", false)
	if err != nil {
		t.Fatalf("unexpected enqueue error: %v", err)
	}
	if j1.ID != j2.ID {
		t.Fatalf("expected idempotent key to return same job ID")
	}
}

func TestQueue_WorkerExecutesPendingJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := NewQueue(16)
	exec := &fakeExecutor{}
	q.StartWorker(ctx, exec)

	job, err := q.Enqueue("ok.yaml", "", false)
	if err != nil {
		t.Fatalf("unexpected enqueue error: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		cur, _ := q.Get(job.ID)
		if cur.Status == JobSucceeded {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for job success; current=%+v", cur)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestQueue_CancelPendingJob(t *testing.T) {
	q := NewQueue(16)
	j, err := q.Enqueue("x.yaml", "", false)
	if err != nil {
		t.Fatalf("unexpected enqueue error: %v", err)
	}
	if err := q.Cancel(j.ID); err != nil {
		t.Fatalf("cancel should succeed: %v", err)
	}
	cur, ok := q.Get(j.ID)
	if !ok || cur.Status != JobCanceled {
		t.Fatalf("expected canceled status, got %+v", cur)
	}
}

func TestQueue_EmergencyStopBlocksNewJobs(t *testing.T) {
	q := NewQueue(8)
	st := q.SetEmergencyStop(true, "incident")
	if !st.Active {
		t.Fatalf("expected emergency stop active")
	}
	if _, err := q.Enqueue("blocked.yaml", "", false); err == nil {
		t.Fatalf("expected enqueue error during emergency stop")
	}
	if _, err := q.Enqueue("forced.yaml", "", true); err != nil {
		t.Fatalf("expected forced enqueue to bypass emergency stop: %v", err)
	}
}
