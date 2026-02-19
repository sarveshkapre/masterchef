package control

import "testing"

func TestCanaryUpgradeStoreRecordAndList(t *testing.T) {
	store := NewCanaryUpgradeStore()
	run, err := store.Record(CanaryUpgradeRun{
		Component:    "control-plane",
		FromChannel:  "stable",
		ToChannel:    "candidate",
		CanaryIDs:    []string{"canary-1"},
		AutoRollback: true,
		Status:       "rolled_back",
		RolledBack:   true,
		Reason:       "canary unhealthy",
	})
	if err != nil {
		t.Fatalf("record run failed: %v", err)
	}
	if run.ID == "" || !run.RolledBack {
		t.Fatalf("unexpected run %+v", run)
	}
	if len(store.List(10)) != 1 {
		t.Fatalf("expected one run")
	}
	if _, ok := store.Get(run.ID); !ok {
		t.Fatalf("expected stored run")
	}
}
