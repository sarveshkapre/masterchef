package state

import (
	"testing"
	"time"
)

func TestStore_SaveAndListRuns(t *testing.T) {
	tmp := t.TempDir()
	s := New(tmp)

	r1 := RunRecord{
		ID:        "r1",
		StartedAt: time.Now().UTC().Add(-time.Minute),
		EndedAt:   time.Now().UTC().Add(-time.Minute + time.Second),
		Status:    RunSucceeded,
	}
	r2 := RunRecord{
		ID:        "r2",
		StartedAt: time.Now().UTC(),
		EndedAt:   time.Now().UTC().Add(time.Second),
		Status:    RunFailed,
	}
	if err := s.SaveRun(r1); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRun(r2); err != nil {
		t.Fatal(err)
	}

	runs, err := s.ListRuns(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	if runs[0].ID != "r2" {
		t.Fatalf("expected newest run first, got %s", runs[0].ID)
	}
}
