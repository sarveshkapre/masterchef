package control

import "testing"

func TestQueueBackendPolicyAndAdmit(t *testing.T) {
	store := NewQueueBackendStore()
	redisBackend, err := store.Upsert(QueueBackendInput{
		Name:    "redis-primary",
		Type:    "redis",
		DSN:     "redis://queue.internal:6379/0",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("upsert redis backend failed: %v", err)
	}
	_, err = store.SetPolicy(QueueBackendPolicyInput{
		ActiveBackendID:    redisBackend.ID,
		FailoverBackendIDs: []string{"queue-backend-1"},
	})
	if err != nil {
		t.Fatalf("set queue backend policy failed: %v", err)
	}
	result := store.Admit(QueueBackendAdmitInput{RequireExternal: true})
	if !result.Allowed || result.SelectedBackend != redisBackend.ID {
		t.Fatalf("expected queue backend admit success, got %+v", result)
	}
}

func TestQueueBackendRejectMissingExternalDSN(t *testing.T) {
	store := NewQueueBackendStore()
	if _, err := store.Upsert(QueueBackendInput{
		Name:    "nats-bad",
		Type:    "nats",
		Enabled: true,
	}); err == nil {
		t.Fatalf("expected missing dsn validation error")
	}
}
