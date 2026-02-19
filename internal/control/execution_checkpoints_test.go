package control

import "testing"

func TestExecutionCheckpointStoreRecordAndList(t *testing.T) {
	store := NewExecutionCheckpointStore()
	first, err := store.Record(ExecutionCheckpointInput{
		RunID:      "run-1",
		JobID:      "job-1",
		ConfigPath: "/tmp/a.yaml",
		StepID:     "a",
		StepOrder:  1,
		Status:     "succeeded",
	})
	if err != nil {
		t.Fatalf("record checkpoint: %v", err)
	}
	if first.ID == "" {
		t.Fatalf("expected checkpoint id")
	}
	list := store.List("run-1", "", 10)
	if len(list) != 1 || list[0].ID != first.ID {
		t.Fatalf("unexpected checkpoint list %+v", list)
	}
	if _, ok := store.Get(first.ID); !ok {
		t.Fatalf("expected get checkpoint by id to succeed")
	}
}
