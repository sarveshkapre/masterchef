package control

import "testing"

func TestAssociationExecutionStoreRecordAndList(t *testing.T) {
	store := NewAssociationExecutionStore(10)

	job := Job{ID: "job-1", ConfigPath: "c.yaml", Priority: "high", Status: JobPending}
	first := store.RecordJob("assoc-1", job)
	if first.AssociationID != "assoc-1" || first.Status != JobPending {
		t.Fatalf("unexpected first execution record: %+v", first)
	}

	job.Status = JobRunning
	second := store.RecordJob("assoc-1", job)
	if second.ID != first.ID || second.Status != JobRunning {
		t.Fatalf("expected record update for same association+job: first=%+v second=%+v", first, second)
	}

	store.RecordJob("assoc-2", Job{ID: "job-2", ConfigPath: "d.yaml", Priority: "normal", Status: JobSucceeded})
	filtered := store.List("assoc-1", 10)
	if len(filtered) != 1 || filtered[0].AssociationID != "assoc-1" {
		t.Fatalf("expected one filtered association execution record, got %+v", filtered)
	}
}
