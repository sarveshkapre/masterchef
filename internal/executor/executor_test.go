package executor

import (
	"os"
	"path/filepath"
	"strings"
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
				Order: 1,
				Host: config.Host{
					Name:      "localhost",
					Transport: "local",
				},
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
				Order: 1,
				Host: config.Host{
					Name:      "localhost",
					Transport: "local",
				},
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

func TestApply_FreeStrategyContinuesAfterFailure(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "after-failure.txt")
	p := &planner.Plan{
		Execution: config.Execution{
			Strategy: "free",
		},
		Steps: []planner.Step{
			{
				Order: 1,
				Host:  config.Host{Name: "localhost", Transport: "local"},
				Resource: config.Resource{
					ID:      "fail-step",
					Type:    "command",
					Host:    "localhost",
					Command: "exit 1",
				},
			},
			{
				Order: 2,
				Host:  config.Host{Name: "localhost", Transport: "local"},
				Resource: config.Resource{
					ID:      "after-step",
					Type:    "file",
					Host:    "localhost",
					Path:    target,
					Content: "ok\n",
				},
			},
		},
	}
	ex := New(tmp)
	run, err := ex.Apply(p)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if run.Status != state.RunFailed {
		t.Fatalf("expected failed run due first step, got %s", run.Status)
	}
	if len(run.Results) != 2 {
		t.Fatalf("expected free strategy to continue, got %d results", len(run.Results))
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected second step to execute and create file: %v", err)
	}
}

func TestApply_AnyErrorsFatalStopsFreeStrategy(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "should-not-exist.txt")
	p := &planner.Plan{
		Execution: config.Execution{
			Strategy:       "free",
			AnyErrorsFatal: true,
		},
		Steps: []planner.Step{
			{
				Order: 1,
				Host:  config.Host{Name: "localhost", Transport: "local"},
				Resource: config.Resource{
					ID:      "fail-step",
					Type:    "command",
					Host:    "localhost",
					Command: "exit 1",
				},
			},
			{
				Order: 2,
				Host:  config.Host{Name: "localhost", Transport: "local"},
				Resource: config.Resource{
					ID:      "after-step",
					Type:    "file",
					Host:    "localhost",
					Path:    target,
					Content: "ok\n",
				},
			},
		},
	}
	ex := New(tmp)
	run, err := ex.Apply(p)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if len(run.Results) != 1 {
		t.Fatalf("expected any_errors_fatal to stop run, got %d", len(run.Results))
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected second step not to run, stat err=%v", err)
	}
}

func TestApply_MaxFailPercentageStopsFreeStrategy(t *testing.T) {
	tmp := t.TempDir()
	p := &planner.Plan{
		Execution: config.Execution{
			Strategy:          "free",
			MaxFailPercentage: 10,
		},
		Steps: []planner.Step{
			{
				Order: 1,
				Host:  config.Host{Name: "localhost", Transport: "local"},
				Resource: config.Resource{
					ID:      "fail-step",
					Type:    "command",
					Host:    "localhost",
					Command: "exit 1",
				},
			},
			{
				Order: 2,
				Host:  config.Host{Name: "localhost", Transport: "local"},
				Resource: config.Resource{
					ID:      "next-step",
					Type:    "command",
					Host:    "localhost",
					Command: "echo should-not-run",
				},
			},
		},
	}
	ex := New(tmp)
	run, err := ex.Apply(p)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if len(run.Results) != 1 {
		t.Fatalf("expected failure percentage policy to stop run, got %d", len(run.Results))
	}
}

func TestApply_CommandRetriesUntilSuccess(t *testing.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, "retry.marker")
	cmd := "if [ ! -f " + marker + " ]; then touch " + marker + "; echo warming-up; exit 1; fi; echo ready"

	p := &planner.Plan{
		Steps: []planner.Step{
			{
				Order: 1,
				Host:  config.Host{Name: "localhost", Transport: "local"},
				Resource: config.Resource{
					ID:                "retry-step",
					Type:              "command",
					Host:              "localhost",
					Command:           cmd,
					Retries:           1,
					RetryDelaySeconds: 0,
				},
			},
		},
	}

	ex := New(tmp)
	run, err := ex.Apply(p)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if run.Status != state.RunSucceeded {
		t.Fatalf("expected succeeded after retry, got %s with results %#v", run.Status, run.Results)
	}
	if len(run.Results) != 1 || !strings.Contains(run.Results[0].Message, "succeeded after 2 attempts") {
		t.Fatalf("expected retry success message, got %#v", run.Results)
	}
}

func TestApply_CommandUntilContains(t *testing.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, "until.marker")
	cmd := "if [ ! -f " + marker + " ]; then touch " + marker + "; echo pending; exit 0; fi; echo done"

	p := &planner.Plan{
		Steps: []planner.Step{
			{
				Order: 1,
				Host:  config.Host{Name: "localhost", Transport: "local"},
				Resource: config.Resource{
					ID:                "until-step",
					Type:              "command",
					Host:              "localhost",
					Command:           cmd,
					UntilContains:     "done",
					Retries:           1,
					RetryDelaySeconds: 0,
				},
			},
		},
	}

	ex := New(tmp)
	run, err := ex.Apply(p)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if run.Status != state.RunSucceeded {
		t.Fatalf("expected until condition to succeed on retry, got %s", run.Status)
	}

	failPlan := &planner.Plan{
		Steps: []planner.Step{
			{
				Order: 1,
				Host:  config.Host{Name: "localhost", Transport: "local"},
				Resource: config.Resource{
					ID:            "until-fail",
					Type:          "command",
					Host:          "localhost",
					Command:       "echo never",
					UntilContains: "always-missing",
					Retries:       1,
				},
			},
		},
	}
	run, err = ex.Apply(failPlan)
	if err != nil {
		t.Fatalf("apply failed unexpectedly: %v", err)
	}
	if run.Status != state.RunFailed {
		t.Fatalf("expected until condition failure, got %s", run.Status)
	}
}

func TestApply_WinRMTransportLocalhostShim(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "winrm-file.txt")
	p := &planner.Plan{
		Steps: []planner.Step{
			{
				Order: 1,
				Host:  config.Host{Name: "localhost", Transport: "winrm"},
				Resource: config.Resource{
					ID:      "f-winrm",
					Type:    "file",
					Host:    "localhost",
					Path:    target,
					Content: "winrm-localhost-shim\n",
				},
			},
			{
				Order: 2,
				Host:  config.Host{Name: "localhost", Transport: "winrm"},
				Resource: config.Resource{
					ID:      "c-winrm",
					Type:    "command",
					Host:    "localhost",
					Command: "echo winrm-command",
				},
			},
		},
	}
	ex := New(tmp)
	run, err := ex.Apply(p)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if run.Status != state.RunSucceeded {
		t.Fatalf("expected winrm localhost shim run to succeed, got %s", run.Status)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected winrm file resource to materialize file: %v", err)
	}
}

func TestApply_CustomTransportPluginHandler(t *testing.T) {
	ex := New("")
	if err := ex.RegisterTransport("plugin/mock", func(step planner.Step, r config.Resource) (bool, bool, string, error) {
		if r.ID != "custom-step" || step.Host.Transport != "plugin/mock" {
			t.Fatalf("unexpected custom transport step: %#v", step)
		}
		return true, false, "mock transport applied", nil
	}); err != nil {
		t.Fatalf("register custom transport failed: %v", err)
	}

	p := &planner.Plan{
		Steps: []planner.Step{
			{
				Order: 1,
				Host:  config.Host{Name: "host-1", Transport: "plugin/mock"},
				Resource: config.Resource{
					ID:   "custom-step",
					Type: "file",
					Host: "host-1",
				},
			},
		},
	}
	run, err := ex.Apply(p)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if run.Status != state.RunSucceeded || len(run.Results) != 1 || !run.Results[0].Changed {
		t.Fatalf("expected successful custom transport run, got %#v", run)
	}
}
