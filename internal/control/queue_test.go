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
	j1 := q.Enqueue("a.yaml", "k1")
	j2 := q.Enqueue("b.yaml", "k1")
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

	job := q.Enqueue("ok.yaml", "")
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
	j := q.Enqueue("x.yaml", "")
	if err := q.Cancel(j.ID); err != nil {
		t.Fatalf("cancel should succeed: %v", err)
	}
	cur, ok := q.Get(j.ID)
	if !ok || cur.Status != JobCanceled {
		t.Fatalf("expected canceled status, got %+v", cur)
	}
}
