package control

import (
	"testing"
	"time"
)

func TestExecutionLockAcquireConflictAndRelease(t *testing.T) {
	store := NewExecutionLockStore()
	lock, err := store.Acquire(ExecutionLockAcquireInput{Key: "env/prod", Holder: "job-a", TTLSeconds: 60})
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	if lock.Status != "active" {
		t.Fatalf("expected active lock, got %+v", lock)
	}
	if _, err := store.Acquire(ExecutionLockAcquireInput{Key: "env/prod", Holder: "job-b", TTLSeconds: 60}); err == nil {
		t.Fatalf("expected lock conflict")
	}
	released, ok := store.Release(ExecutionLockReleaseInput{Key: "env/prod"})
	if !ok || released.Status != "released" {
		t.Fatalf("expected released lock, got ok=%v lock=%+v", ok, released)
	}
}

func TestExecutionLockBindAndCleanupExpired(t *testing.T) {
	store := NewExecutionLockStore()
	lock, err := store.Acquire(ExecutionLockAcquireInput{Key: "service/api", Holder: "job-a", TTLSeconds: 1})
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	if _, err := store.BindJob(lock.Key, "job-123"); err != nil {
		t.Fatalf("bind job: %v", err)
	}
	if _, ok := store.Release(ExecutionLockReleaseInput{JobID: "job-123"}); !ok {
		t.Fatalf("expected release by job to succeed")
	}

	_, err = store.Acquire(ExecutionLockAcquireInput{Key: "service/db", Holder: "job-x", TTLSeconds: 1})
	if err != nil {
		t.Fatalf("acquire short lock: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	expired := store.CleanupExpired()
	if len(expired) == 0 {
		t.Fatalf("expected expired lock cleanup")
	}
}
