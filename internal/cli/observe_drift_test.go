package cli

import (
	"testing"
	"time"

	"github.com/masterchef/masterchef/internal/state"
)

func TestRunObserveAndDrift(t *testing.T) {
	tmp := t.TempDir()
	st := state.New(tmp)
	now := time.Now().UTC()

	if err := st.SaveRun(state.RunRecord{
		ID:        "run-1",
		StartedAt: now.Add(-30 * time.Minute),
		EndedAt:   now.Add(-29 * time.Minute),
		Status:    state.RunSucceeded,
		Results: []state.ResourceRun{
			{ResourceID: "f1", Type: "file", Host: "web-01", Changed: true},
			{ResourceID: "svc", Type: "service", Host: "web-01", Changed: false, Skipped: true},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := st.SaveRun(state.RunRecord{
		ID:        "run-2",
		StartedAt: now.Add(-10 * time.Minute),
		EndedAt:   now.Add(-9 * time.Minute),
		Status:    state.RunFailed,
		Results: []state.ResourceRun{
			{ResourceID: "cmd", Type: "command", Host: "db-01", Changed: true},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := runObserve([]string{"-base", tmp, "-limit", "10", "-format", "json"}); err != nil {
		t.Fatalf("runObserve failed: %v", err)
	}
	if err := runDrift([]string{"-base", tmp, "-hours", "24", "-format", "json"}); err != nil {
		t.Fatalf("runDrift failed: %v", err)
	}
}
