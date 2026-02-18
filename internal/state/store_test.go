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

	got, err := s.GetRun("r1")
	if err != nil {
		t.Fatalf("expected get run to succeed: %v", err)
	}
	if got.ID != "r1" {
		t.Fatalf("expected run id r1, got %s", got.ID)
	}

	replacement := []RunRecord{
		{
			ID:        "r3",
			StartedAt: time.Now().UTC(),
			EndedAt:   time.Now().UTC().Add(time.Second),
			Status:    RunSucceeded,
		},
	}
	if err := s.ReplaceRuns(replacement); err != nil {
		t.Fatalf("expected replace runs to succeed: %v", err)
	}
	runs, err = s.ListRuns(10)
	if err != nil {
		t.Fatalf("list runs after replace failed: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "r3" {
		t.Fatalf("expected replacement runs only, got %+v", runs)
	}
}
