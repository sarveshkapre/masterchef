package checker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/planner"
)

func TestRun_Report(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "a.txt")
	if err := os.WriteFile(f, []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &planner.Plan{
		Steps: []planner.Step{
			{
				Order: 1,
				Host:  config.Host{Name: "localhost", Transport: "local"},
				Resource: config.Resource{
					ID:      "file1",
					Type:    "file",
					Host:    "localhost",
					Path:    f,
					Content: "same",
				},
			},
			{
				Order: 2,
				Host:  config.Host{Name: "n1", Transport: "ssh"},
				Resource: config.Resource{
					ID:      "cmd1",
					Type:    "command",
					Host:    "n1",
					Command: "echo hi",
				},
			},
		},
	}
	r := Run(p)
	if r.TotalResources != 2 {
		t.Fatalf("unexpected total resources: %d", r.TotalResources)
	}
	if r.Simulatable != 1 || r.NonSimulatable != 1 {
		t.Fatalf("unexpected simulation counts: %+v", r)
	}
	if r.Confidence != 0.5 {
		t.Fatalf("unexpected confidence: %f", r.Confidence)
	}
}

func TestRun_FileDiffPatch(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "a.txt")
	if err := os.WriteFile(f, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &planner.Plan{
		Steps: []planner.Step{
			{
				Order: 1,
				Host:  config.Host{Name: "localhost", Transport: "local"},
				Resource: config.Resource{
					ID:      "file1",
					Type:    "file",
					Host:    "localhost",
					Path:    f,
					Content: "after\n",
				},
			},
		},
	}
	r := Run(p)
	if len(r.Items) != 1 || r.Items[0].Diff == "" {
		t.Fatalf("expected patch diff in report: %+v", r)
	}
}

func TestRun_WinRMSimulation(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "winrm.txt")
	p := &planner.Plan{
		Steps: []planner.Step{
			{
				Order: 1,
				Host:  config.Host{Name: "win-host", Transport: "winrm"},
				Resource: config.Resource{
					ID:      "file-winrm",
					Type:    "file",
					Host:    "win-host",
					Path:    f,
					Content: "hello",
				},
			},
		},
	}
	r := Run(p)
	if r.Simulatable != 1 || r.NonSimulatable != 0 {
		t.Fatalf("expected winrm to be simulatable, got %+v", r)
	}
}

func TestRun_CommandOnlyIfGuard(t *testing.T) {
	p := &planner.Plan{
		Steps: []planner.Step{
			{
				Order: 1,
				Host:  config.Host{Name: "localhost", Transport: "local"},
				Resource: config.Resource{
					ID:      "cmd-only-if",
					Type:    "command",
					Host:    "localhost",
					Command: "echo should-not-run",
					OnlyIf:  "exit 1",
				},
			},
		},
	}
	r := Run(p)
	if len(r.Items) != 1 {
		t.Fatalf("expected one report item, got %+v", r)
	}
	if r.Items[0].WouldChange {
		t.Fatalf("expected only_if failure to skip simulated change, got %+v", r.Items[0])
	}
	if r.Items[0].Reason != "only_if condition failed" {
		t.Fatalf("unexpected reason %q", r.Items[0].Reason)
	}
}
