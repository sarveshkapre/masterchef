package executor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/planner"
	"github.com/masterchef/masterchef/internal/state"
)

func TestApply_FileIsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "demo.txt")

	p := &planner.Plan{
		Steps: []planner.Step{
			{
				Order:         1,
				HostTransport: "local",
				Resource: config.Resource{
					ID:      "f1",
					Type:    "file",
					Host:    "localhost",
					Path:    target,
					Content: "hello\n",
				},
			},
		},
	}

	ex := New(tmp)
	r1, err := ex.Apply(p)
	if err != nil {
		t.Fatalf("first apply failed: %v", err)
	}
	if r1.Status != state.RunSucceeded || len(r1.Results) != 1 || !r1.Results[0].Changed {
		t.Fatalf("unexpected first run result: %#v", r1)
	}

	r2, err := ex.Apply(p)
	if err != nil {
		t.Fatalf("second apply failed: %v", err)
	}
	if r2.Results[0].Changed {
		t.Fatalf("expected idempotent second run, got changed=true")
	}
}

func TestApply_CommandCreatesSkipsSecondRun(t *testing.T) {
	tmp := t.TempDir()
	creates := filepath.Join(tmp, "created.flag")
	cmd := "touch " + creates

	p := &planner.Plan{
		Steps: []planner.Step{
			{
				Order:         1,
				HostTransport: "local",
				Resource: config.Resource{
					ID:      "c1",
					Type:    "command",
					Host:    "localhost",
					Command: cmd,
					Creates: creates,
				},
			},
		},
	}

	ex := New(tmp)
	r1, err := ex.Apply(p)
	if err != nil {
		t.Fatalf("first command apply failed: %v", err)
	}
	if r1.Results[0].Skipped {
		t.Fatalf("first run should not be skipped")
	}
	if _, err := os.Stat(creates); err != nil {
		t.Fatalf("expected creates file to exist: %v", err)
	}

	r2, err := ex.Apply(p)
	if err != nil {
		t.Fatalf("second command apply failed: %v", err)
	}
	if !r2.Results[0].Skipped {
		t.Fatalf("expected second run to be skipped")
	}
}
