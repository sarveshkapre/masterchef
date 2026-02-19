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
	j1, err := q.Enqueue("a.yaml", "k1", false, "")
	if err != nil {
		t.Fatalf("unexpected enqueue error: %v", err)
	}
	j2, err := q.Enqueue("b.yaml", "k1", false, "")
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

	job, err := q.Enqueue("ok.yaml", "", false, "")
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
	j, err := q.Enqueue("x.yaml", "", false, "")
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
	if _, err := q.Enqueue("blocked.yaml", "", false, ""); err == nil {
		t.Fatalf("expected enqueue error during emergency stop")
	}
	if _, err := q.Enqueue("forced.yaml", "", true, ""); err != nil {
		t.Fatalf("expected forced enqueue to bypass emergency stop: %v", err)
	}
}

func TestQueue_PauseResumeAndControlStatus(t *testing.T) {
	q := NewQueue(8)
	st := q.Pause()
	if !st.Paused {
		t.Fatalf("expected paused state")
	}
	st = q.Resume()
	if st.Paused {
		t.Fatalf("expected resumed state")
	}
}

func TestQueue_ChangeFreezeBlocksUnlessForced(t *testing.T) {
	q := NewQueue(8)
	st := q.SetFreezeUntil(time.Now().UTC().Add(2*time.Minute), "release freeze")
	if !st.Active {
		t.Fatalf("expected freeze to be active")
	}
	if _, err := q.Enqueue("blocked.yaml", "", false, ""); err == nil {
		t.Fatalf("expected enqueue to fail during freeze")
	}
	if _, err := q.Enqueue("forced.yaml", "", true, ""); err != nil {
		t.Fatalf("expected forced enqueue to bypass freeze: %v", err)
	}
	st = q.ClearFreeze()
	if st.Active {
		t.Fatalf("expected freeze to be cleared")
	}
}

func TestQueue_SafeDrain_NoRunningJobs(t *testing.T) {
	q := NewQueue(8)
	st, err := q.SafeDrain(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected safe-drain error: %v", err)
	}
	if !st.Paused {
		t.Fatalf("safe-drain should pause queue")
	}
}

func TestQueue_RecoverStuckJobs(t *testing.T) {
	q := NewQueue(8)
	j, err := q.Enqueue("x.yaml", "", false, "")
	if err != nil {
		t.Fatalf("unexpected enqueue error: %v", err)
	}

	q.mu.Lock()
	q.jobs[j.ID].Status = JobRunning
	q.jobs[j.ID].StartedAt = time.Now().UTC().Add(-10 * time.Minute)
	q.running = 1
	q.mu.Unlock()

	recovered := q.RecoverStuckJobs(30 * time.Second)
	if len(recovered) != 1 {
		t.Fatalf("expected one recovered job, got %d", len(recovered))
	}
	cur, _ := q.Get(j.ID)
	if cur.Status != JobFailed {
		t.Fatalf("expected recovered job to be failed, got %s", cur.Status)
	}
}

func TestQueue_PriorityNormalizationAndStatusBreakdown(t *testing.T) {
	q := NewQueue(8)
	j1, err := q.Enqueue("a.yaml", "", false, "HIGH")
	if err != nil {
		t.Fatalf("unexpected enqueue error: %v", err)
	}
	j2, err := q.Enqueue("b.yaml", "", false, "low")
	if err != nil {
		t.Fatalf("unexpected enqueue error: %v", err)
	}
	j3, err := q.Enqueue("c.yaml", "", false, "unknown")
	if err != nil {
		t.Fatalf("unexpected enqueue error: %v", err)
	}
	if j1.Priority != "high" || j2.Priority != "low" || j3.Priority != "normal" {
		t.Fatalf("priority normalization mismatch: %s %s %s", j1.Priority, j2.Priority, j3.Priority)
	}
	st := q.ControlStatus()
	if st.PendingHigh != 1 || st.PendingNormal != 1 || st.PendingLow != 1 || st.Pending != 3 {
		t.Fatalf("unexpected pending breakdown: %+v", st)
	}
}

func TestQueue_WorkerLifecyclePolicyStateless(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := NewQueue(16)
	policy := q.SetWorkerLifecyclePolicy(WorkerLifecycleInput{
		Mode:             "stateless",
		MaxJobsPerWorker: 1,
	})
	if policy.Mode != "stateless" || policy.MaxJobsPerWorker != 1 {
		t.Fatalf("unexpected lifecycle policy %+v", policy)
	}

	exec := &fakeExecutor{}
	q.StartWorker(ctx, exec)
	j1, err := q.Enqueue("a.yaml", "", false, "")
	if err != nil {
		t.Fatalf("enqueue a.yaml: %v", err)
	}
	j2, err := q.Enqueue("b.yaml", "", false, "")
	if err != nil {
		t.Fatalf("enqueue b.yaml: %v", err)
	}

	waitSucceeded := func(id string) {
		deadline := time.Now().Add(2 * time.Second)
		for {
			cur, _ := q.Get(id)
			if cur != nil && cur.Status == JobSucceeded {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for job %s success; current=%+v", id, cur)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitSucceeded(j1.ID)
	waitSucceeded(j2.ID)

	status := q.WorkerLifecycleStatus()
	if status.Recycles < 2 {
		t.Fatalf("expected at least 2 worker recycles in stateless mode, got %+v", status)
	}
}

func TestQueue_FailJob(t *testing.T) {
	q := NewQueue(8)
	job, err := q.Enqueue("fail.yaml", "", false, "")
	if err != nil {
		t.Fatalf("enqueue fail.yaml: %v", err)
	}
	failed, err := q.FailJob(job.ID, "lease expired")
	if err != nil {
		t.Fatalf("fail job: %v", err)
	}
	if failed.Status != JobFailed || failed.Error != "lease expired" {
		t.Fatalf("expected failed lease-expired job, got %+v", failed)
	}
}
