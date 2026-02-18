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
