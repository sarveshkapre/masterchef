package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/masterchef/masterchef/internal/state"
)

func TestRunTUIWithIO_InteractiveSelection(t *testing.T) {
	tmp := t.TempDir()
	store := state.New(tmp)
	run := state.RunRecord{
		ID:        "run-1",
		StartedAt: time.Now().UTC().Add(-1 * time.Minute),
		EndedAt:   time.Now().UTC(),
		Status:    state.RunSucceeded,
		Results: []state.ResourceRun{
			{
				ResourceID: "f1",
				Type:       "file",
				Host:       "localhost",
				Changed:    true,
				Message:    "updated",
			},
		},
	}
	if err := store.SaveRun(run); err != nil {
		t.Fatalf("save run failed: %v", err)
	}

	var out bytes.Buffer
	in := strings.NewReader("1\nq\n")
	if err := runTUIWithIO([]string{"-base", tmp, "-limit", "10"}, in, &out); err != nil {
		t.Fatalf("runTUIWithIO failed: %v", err)
	}
	body := out.String()
	if !strings.Contains(body, "Masterchef Run Inspector") {
		t.Fatalf("expected inspector header in output: %s", body)
	}
	if !strings.Contains(body, "Run run-1 (succeeded)") {
		t.Fatalf("expected selected run details in output: %s", body)
	}
	if !strings.Contains(body, "- f1 host=localhost type=file changed=true skipped=false") {
		t.Fatalf("expected resource row in output: %s", body)
	}
}

func TestRunTUIWithIO_NoRuns(t *testing.T) {
	tmp := t.TempDir()
	var out bytes.Buffer
	if err := runTUIWithIO([]string{"-base", tmp}, strings.NewReader("q\n"), &out); err != nil {
		t.Fatalf("runTUIWithIO failed: %v", err)
	}
	if !strings.Contains(out.String(), "tui: no runs found") {
		t.Fatalf("expected no-runs message, got %s", out.String())
	}
}
