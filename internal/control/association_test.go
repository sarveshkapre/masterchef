package control

import (
	"testing"
	"time"
)

func TestAssociationStore_CreateRevisionsAndReplay(t *testing.T) {
	q := NewQueue(32)
	sched := NewScheduler(q)
	store := NewAssociationStore(sched)

	assoc, err := store.Create(AssociationCreate{
		ConfigPath: "x.yaml",
		TargetKind: "environment",
		TargetName: "prod",
		Priority:   "high",
		Interval:   30 * time.Second,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("unexpected create error: %v", err)
	}
	if assoc.ID == "" || assoc.Revision != 1 {
		t.Fatalf("unexpected association create result: %+v", assoc)
	}

	rev, err := store.Revisions(assoc.ID)
	if err != nil {
		t.Fatalf("unexpected revisions error: %v", err)
	}
	if len(rev) != 1 {
		t.Fatalf("expected one initial revision, got %d", len(rev))
	}

	assoc, err = store.SetEnabled(assoc.ID, false)
	if err != nil {
		t.Fatalf("unexpected disable error: %v", err)
	}
	if assoc.Enabled {
		t.Fatalf("expected disabled association")
	}
	if assoc.Revision != 2 {
		t.Fatalf("expected revision 2 after disable, got %d", assoc.Revision)
	}

	assoc, err = store.Replay(assoc.ID, 1)
	if err != nil {
		t.Fatalf("unexpected replay error: %v", err)
	}
	if !assoc.Enabled {
		t.Fatalf("expected replay to restore enabled state from revision 1")
	}
	if assoc.Revision != 3 {
		t.Fatalf("expected revision 3 after replay, got %d", assoc.Revision)
	}
}
