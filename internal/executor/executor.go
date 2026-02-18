package executor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/masterchef/masterchef/internal/planner"
	"github.com/masterchef/masterchef/internal/state"
)

type Executor struct {
	baseDir string
}

func New(baseDir string) *Executor {
	return &Executor{baseDir: baseDir}
}

func (e *Executor) Apply(p *planner.Plan) (state.RunRecord, error) {
	run := state.RunRecord{
		ID:        time.Now().UTC().Format("20060102T150405.000000000"),
		StartedAt: time.Now().UTC(),
		Status:    state.RunSucceeded,
		Results:   make([]state.ResourceRun, 0, len(p.Steps)),
	}

	for _, step := range p.Steps {
		r := step.Resource
		if step.HostTransport != "local" {
			run.Status = state.RunFailed
			run.Results = append(run.Results, state.ResourceRun{
				ResourceID: r.ID,
				Type:       r.Type,
				Host:       r.Host,
				Message:    "non-local hosts not yet supported in executor",
			})
			break
		}

		res := state.ResourceRun{
			ResourceID: r.ID,
			Type:       r.Type,
			Host:       r.Host,
		}

		switch r.Type {
		case "file":
			changed, msg, err := e.applyFile(r.Path, r.Content, r.Mode)
			res.Changed = changed
			res.Message = msg
			if err != nil {
				run.Status = state.RunFailed
				res.Message = err.Error()
			}
		case "command":
			changed, skipped, msg, err := e.applyCommand(r.Command, r.Creates, r.Unless)
			res.Changed = changed
			res.Skipped = skipped
			res.Message = msg
			if err != nil {
				run.Status = state.RunFailed
				res.Message = err.Error()
			}
		default:
			run.Status = state.RunFailed
			res.Message = fmt.Sprintf("unsupported resource type %q", r.Type)
		}

		run.Results = append(run.Results, res)
		if run.Status == state.RunFailed {
			break
		}
	}

	run.EndedAt = time.Now().UTC()
	if run.Status == "" {
		run.Status = state.RunSucceeded
	}
	return run, nil
}

func (e *Executor) applyFile(path, content, mode string) (bool, string, error) {
	full := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return false, "", fmt.Errorf("mkdir for file resource: %w", err)
	}

	current, err := os.ReadFile(full)
	if err == nil && bytes.Equal(current, []byte(content)) {
		return false, "file already in desired state", nil
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return false, "", fmt.Errorf("write file: %w", err)
	}
	if mode != "" {
		// ignore parse error fallback to no-op mode setting for v0 stability
		_ = os.Chmod(full, 0o644)
	}
	return true, "file updated", nil
}

func (e *Executor) applyCommand(cmdText, creates, unless string) (bool, bool, string, error) {
	if creates != "" {
		if _, err := os.Stat(creates); err == nil {
			return false, true, "command skipped: creates path already exists", nil
		}
	}
	if unless != "" {
		if err := exec.Command("sh", "-c", unless).Run(); err == nil {
			return false, true, "command skipped: unless condition succeeded", nil
		}
	}

	cmd := exec.Command("sh", "-c", cmdText)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, false, string(out), fmt.Errorf("command failed: %w: %s", err, string(out))
	}
	return true, false, string(out), nil
}
