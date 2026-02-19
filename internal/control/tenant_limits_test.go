package control

import "testing"

func TestTenantLimitStorePolicyAndAdmit(t *testing.T) {
	store := NewTenantLimitStore()
	policy, err := store.Upsert(TenantLimitPolicyInput{
		Tenant:               "tenant-a",
		RequestsPerMinute:    120,
		MaxConcurrentRuns:    10,
		MaxQueueSharePercent: 40,
		Burst:                20,
	})
	if err != nil {
		t.Fatalf("upsert tenant policy failed: %v", err)
	}
	if policy.ID == "" {
		t.Fatalf("expected policy id")
	}
	ok := store.Admit(TenantAdmissionInput{Tenant: "tenant-a", RequestedRuns: 1, CurrentRuns: 3, QueueDepth: 100, TenantQueued: 20})
	if !ok.Allowed {
		t.Fatalf("expected admit allowed, got %+v", ok)
	}
	badConcurrent := store.Admit(TenantAdmissionInput{Tenant: "tenant-a", RequestedRuns: 3, CurrentRuns: 9, QueueDepth: 100, TenantQueued: 20})
	if badConcurrent.Allowed {
		t.Fatalf("expected concurrent limit rejection, got %+v", badConcurrent)
	}
	badShare := store.Admit(TenantAdmissionInput{Tenant: "tenant-a", RequestedRuns: 1, CurrentRuns: 3, QueueDepth: 100, TenantQueued: 60})
	if badShare.Allowed {
		t.Fatalf("expected queue-share rejection, got %+v", badShare)
	}
}
