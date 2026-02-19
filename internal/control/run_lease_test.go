package control

import (
	"testing"
	"time"
)

func TestRunLeaseAcquireHeartbeatRelease(t *testing.T) {
	store := NewRunLeaseStore()
	lease, err := store.Acquire(RunLeaseAcquireInput{JobID: "job-1", Holder: "worker-a", TTLSeconds: 1})
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	if lease.LeaseID == "" || lease.Status != "active" {
		t.Fatalf("unexpected lease %+v", lease)
	}
	hb, err := store.Heartbeat(RunLeaseHeartbeatInput{LeaseID: lease.LeaseID})
	if err != nil {
		t.Fatalf("lease heartbeat: %v", err)
	}
	if hb.ExpiresAt.Before(time.Now().UTC()) {
		t.Fatalf("expected heartbeat to extend expiry")
	}
	released, err := store.Release(RunLeaseHeartbeatInput{JobID: "job-1"})
	if err != nil {
		t.Fatalf("release lease: %v", err)
	}
	if released.Status != "released" {
		t.Fatalf("expected released status, got %+v", released)
	}
}

func TestRunLeaseRecoverExpired(t *testing.T) {
	store := NewRunLeaseStore()
	lease, err := store.Acquire(RunLeaseAcquireInput{JobID: "job-expired", Holder: "worker-b", TTLSeconds: 1})
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	recovered := store.RecoverExpired(lease.ExpiresAt.Add(10 * time.Millisecond))
	if len(recovered) != 1 {
		t.Fatalf("expected one recovered lease, got %d", len(recovered))
	}
	if recovered[0].Status != "recovered" {
		t.Fatalf("expected recovered status, got %+v", recovered[0])
	}
}
