package executor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
					ID:         "custom-step",
					Type:       "command",
					Host:       "host-1",
					Command:    "echo ok",
					Become:     true,
					BecomeUser: "ops",
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
	if !strings.Contains(run.Results[0].Message, "privilege escalation via sudo as ops") {
		t.Fatalf("expected privilege escalation audit marker, got %q", run.Results[0].Message)
	}
}

func TestBuildSSHArgs_WithJumpHostAndProxyCommand(t *testing.T) {
	ex := New("")
	host := config.Host{
		Name:         "app-1",
		Transport:    "ssh",
		Address:      "10.0.0.10",
		User:         "ubuntu",
		Port:         2222,
		JumpAddress:  "bastion.internal",
		JumpUser:     "ops",
		JumpPort:     2200,
		ProxyCommand: "nc -x proxy.internal:1080 %h %p",
	}
	args := ex.buildSSHArgs(host, "echo ready")
	want := []string{
		"-p", "2222",
		"-J", "ops@bastion.internal:2200",
		"-o", "ProxyCommand=nc -x proxy.internal:1080 %h %p",
		"ubuntu@10.0.0.10",
		"sh", "-lc", "echo ready",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected ssh args:\nwant: %#v\ngot:  %#v", want, args)
	}
}

func TestBuildSSHJumpTarget(t *testing.T) {
	if got := buildSSHJumpTarget(config.Host{}); got != "" {
		t.Fatalf("expected empty jump target, got %q", got)
	}
	host := config.Host{JumpAddress: "bastion", JumpUser: "ops", JumpPort: 2022}
	if got := buildSSHJumpTarget(host); got != "ops@bastion:2022" {
		t.Fatalf("unexpected jump target %q", got)
	}
}

func TestPrepareResourceForExecution_SudoWrap(t *testing.T) {
	resource := config.Resource{
		ID:         "c1",
		Type:       "command",
		Command:    "echo ok",
		Unless:     "test -f /tmp/skip",
		Become:     true,
		BecomeUser: "root",
	}
	prepared, audit, err := prepareResourceForExecution(config.Host{Transport: "ssh"}, resource)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if !strings.Contains(prepared.Command, "sudo -u 'root' sh -lc ") {
		t.Fatalf("expected command to be wrapped with sudo, got %q", prepared.Command)
	}
	if !strings.Contains(prepared.Unless, "sudo -u 'root' sh -lc ") {
		t.Fatalf("expected unless to be wrapped with sudo, got %q", prepared.Unless)
	}
	if audit != "privilege escalation via sudo as root" {
		t.Fatalf("unexpected audit marker %q", audit)
	}
}

func TestPrepareResourceForExecution_WinRMBecomeUnsupported(t *testing.T) {
	_, _, err := prepareResourceForExecution(
		config.Host{Transport: "winrm"},
		config.Resource{ID: "c1", Type: "command", Command: "Write-Output ok", Become: true},
	)
	if err == nil {
		t.Fatalf("expected winrm become to fail")
	}
}

func TestApply_PrivilegedRemoteSessionRecording(t *testing.T) {
	tmp := t.TempDir()
	ex := New(tmp)
	if err := ex.RegisterTransport("ssh", func(step planner.Step, r config.Resource) (bool, bool, string, error) {
		if !strings.Contains(r.Command, "sudo sh -lc ") {
			t.Fatalf("expected sudo wrapping for ssh become command, got %q", r.Command)
		}
		return true, false, "applied", nil
	}); err != nil {
		t.Fatalf("register ssh transport override failed: %v", err)
	}

	p := &planner.Plan{
		Steps: []planner.Step{
			{
				Order: 1,
				Host:  config.Host{Name: "edge-1", Transport: "ssh"},
				Resource: config.Resource{
					ID:      "priv-remote",
					Type:    "command",
					Host:    "edge-1",
					Command: "echo secure",
					Become:  true,
				},
			},
		},
	}
	run, err := ex.Apply(p)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if run.Status != state.RunSucceeded || len(run.Results) != 1 {
		t.Fatalf("unexpected run result %#v", run)
	}
	if !strings.Contains(run.Results[0].Message, "session record: ") {
		t.Fatalf("expected session record path in message, got %q", run.Results[0].Message)
	}

	sessionDir := filepath.Join(tmp, ".masterchef", "sessions")
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("read session dir failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one session record, got %d", len(entries))
	}
	body, err := os.ReadFile(filepath.Join(sessionDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read session file failed: %v", err)
	}
	var rec sessionRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		t.Fatalf("decode session record failed: %v", err)
	}
	if rec.Resource != "priv-remote" || rec.Transport != "ssh" || !rec.Become {
		t.Fatalf("unexpected session record %+v", rec)
	}
}
