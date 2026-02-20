package control

import (
	"testing"
	"time"
)

func TestStuckRecoveryStorePolicyAndStatus(t *testing.T) {
	store := NewStuckRecoveryStore()

	policy, err := store.SetPolicy(StuckRecoveryPolicy{
		Enabled:         true,
		MaxAgeSeconds:   120,
		CooldownSeconds: 10,
	})
	if err != nil {
		t.Fatalf("set policy failed: %v", err)
	}
	if policy.MaxAgeSeconds != 120 || policy.CooldownSeconds != 10 {
		t.Fatalf("unexpected policy: %+v", policy)
	}
	if _, err := store.SetPolicy(StuckRecoveryPolicy{
		Enabled:         true,
		MaxAgeSeconds:   0,
		CooldownSeconds: 10,
	}); err == nil {
		t.Fatalf("expected invalid max_age_seconds failure")
	}

	now := time.Now().UTC()
	got, ok := store.PrepareAutoRun(now)
	if !ok || got.MaxAgeSeconds != 120 {
		t.Fatalf("expected first auto prepare success, got policy=%+v ok=%t", got, ok)
	}
	if _, ok := store.PrepareAutoRun(now.Add(2 * time.Second)); ok {
		t.Fatalf("expected cooldown to block immediate auto run")
	}
	if _, ok := store.PrepareAutoRun(now.Add(12 * time.Second)); !ok {
		t.Fatalf("expected auto run to pass after cooldown")
	}

	store.RecordAutoRunResult(now.Add(12*time.Second), 2)
	store.RecordManualRun(now.Add(30*time.Second), 90, 1)
	status := store.Status()
	if status.TotalRecovered != 3 || status.LastMode != "manual" || status.LastMaxAgeSeconds != 90 {
		t.Fatalf("unexpected status after runs: %+v", status)
	}
	if status.LastRecoveredAt.IsZero() || status.LastCheckAt.IsZero() {
		t.Fatalf("expected last timestamps to be populated, got %+v", status)
	}
}
