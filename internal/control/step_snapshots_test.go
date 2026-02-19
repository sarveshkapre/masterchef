package control

import (
	"testing"
	"time"
)

func TestStepSnapshotStoreRecordAndQuery(t *testing.T) {
	store := NewStepSnapshotStore(100)
	start := time.Now().UTC().Add(-2 * time.Second)
	end := time.Now().UTC()
	item, err := store.Record(StepSnapshotInput{
		RunID:        "run-1",
		JobID:        "job-1",
		StepID:       "pkg-install",
		ResourceType: "package",
		Host:         "web-1",
		Status:       "succeeded",
		StartedAt:    start.Format(time.RFC3339),
		EndedAt:      end.Format(time.RFC3339),
		StdoutHash:   "sha256:aaa",
		Metadata:     map[string]string{"attempt": "1"},
	})
	if err != nil {
		t.Fatalf("record step snapshot: %v", err)
	}
	if item.SnapshotID == "" || item.DurationMS == 0 {
		t.Fatalf("unexpected snapshot %+v", item)
	}
	results := store.List(StepSnapshotQuery{RunID: "run-1", Limit: 10})
	if len(results) != 1 || results[0].SnapshotID != item.SnapshotID {
		t.Fatalf("unexpected snapshot query results %+v", results)
	}
}

func TestStepSnapshotStoreRejectInvalidStatus(t *testing.T) {
	store := NewStepSnapshotStore(10)
	if _, err := store.Record(StepSnapshotInput{StepID: "x", Status: "unknown"}); err == nil {
		t.Fatalf("expected invalid status to fail")
	}
}
