package control

import (
	"testing"
	"time"
)

func TestQueueBacklogSLOStorePolicyAndHistory(t *testing.T) {
	store := NewQueueBacklogSLOStore(25, 3)

	policy, err := store.SetPolicy(QueueBacklogSLOPolicy{
		Threshold:         40,
		WarningPercent:    65,
		RecoveryPercent:   45,
		ProjectionSeconds: 600,
	})
	if err != nil {
		t.Fatalf("set policy failed: %v", err)
	}
	if policy.Threshold != 40 || policy.WarningPercent != 65 {
		t.Fatalf("unexpected policy: %+v", policy)
	}
	if _, err := store.SetPolicy(QueueBacklogSLOPolicy{
		Threshold:         1,
		WarningPercent:    50,
		RecoveryPercent:   50,
		ProjectionSeconds: 300,
	}); err == nil {
		t.Fatalf("expected invalid recovery_percent policy error")
	}

	base := time.Now().UTC()
	store.Record(QueueBacklogSLOStatus{At: base.Add(-3 * time.Second), Pending: 10, Threshold: 40, State: "normal"})
	store.Record(QueueBacklogSLOStatus{At: base.Add(-2 * time.Second), Pending: 35, Threshold: 40, State: "warning"})
	latest := store.Record(QueueBacklogSLOStatus{At: base.Add(-1 * time.Second), Pending: 42, Threshold: 40, State: "saturated"})

	gotLatest, ok := store.Latest()
	if !ok {
		t.Fatalf("expected latest status")
	}
	if gotLatest.State != "saturated" || gotLatest.Pending != 42 {
		t.Fatalf("unexpected latest status: %+v", gotLatest)
	}
	if gotLatest.At.IsZero() || latest.At.IsZero() {
		t.Fatalf("expected UTC timestamps in latest status")
	}

	history := store.History(10)
	if len(history) != 3 {
		t.Fatalf("expected 3 status samples, got %d", len(history))
	}
	if history[0].State != "saturated" || history[2].State != "normal" {
		t.Fatalf("expected newest-first history ordering, got %+v", history)
	}
}
