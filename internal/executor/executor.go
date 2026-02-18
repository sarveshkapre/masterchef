package executor

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/planner"
	"github.com/masterchef/masterchef/internal/provider"
	"github.com/masterchef/masterchef/internal/state"
)

type Executor struct {
	stepTimeout time.Duration
	registry    *provider.Registry
}

func New(_ string) *Executor {
	return &Executor{
		stepTimeout: 30 * time.Second,
		registry:    provider.NewBuiltinRegistry(),
	}
}

func NewWithRegistry(stepTimeout time.Duration, reg *provider.Registry) *Executor {
	if stepTimeout <= 0 {
		stepTimeout = 30 * time.Second
	}
	if reg == nil {
		reg = provider.NewBuiltinRegistry()
	}
	return &Executor{
		stepTimeout: stepTimeout,
		registry:    reg,
	}
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
		if step.Host.Transport != "local" && step.Host.Transport != "ssh" {
			run.Status = state.RunFailed
			run.Results = append(run.Results, state.ResourceRun{
				ResourceID: r.ID,
				Type:       r.Type,
				Host:       r.Host,
				Message:    "unsupported transport: " + step.Host.Transport,
			})
			break
		}

		res := state.ResourceRun{
			ResourceID: r.ID,
			Type:       r.Type,
			Host:       r.Host,
		}

		if step.Host.Transport == "ssh" {
			changed, skipped, msg, err := e.applyOverSSH(step, r)
			res.Changed = changed
			res.Skipped = skipped
			res.Message = msg
			if err != nil {
				run.Status = state.RunFailed
				res.Message = err.Error()
			}
		} else {
			h, ok := e.registry.Lookup(r.Type)
			if !ok {
				run.Status = state.RunFailed
				res.Message = fmt.Sprintf("no provider registered for type %q", r.Type)
				run.Results = append(run.Results, res)
				break
			}

			ctx, cancel := context.WithTimeout(context.Background(), e.stepTimeout)
			pRes, err := h.Apply(ctx, r)
			cancel()
			res.Changed = pRes.Changed
			res.Skipped = pRes.Skipped
			res.Message = pRes.Message
			if err != nil {
				run.Status = state.RunFailed
				res.Message = err.Error()
			}
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

func (e *Executor) applyOverSSH(step planner.Step, r config.Resource) (bool, bool, string, error) {
	switch r.Type {
	case "file":
		marker := "MASTERCHEF_EOF_" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
		var b strings.Builder
		b.WriteString("mkdir -p ")
		b.WriteString(shellQuote(filepath.Dir(r.Path)))
		b.WriteString("\ncat > ")
		b.WriteString(shellQuote(r.Path))
		b.WriteString(" <<'")
		b.WriteString(marker)
		b.WriteString("'\n")
		b.WriteString(r.Content)
		if !strings.HasSuffix(r.Content, "\n") {
			b.WriteString("\n")
		}
		b.WriteString(marker)
		b.WriteString("\n")
		if r.Mode != "" {
			b.WriteString("chmod ")
			b.WriteString(shellQuote(r.Mode))
			b.WriteString(" ")
			b.WriteString(shellQuote(r.Path))
			b.WriteString("\n")
		}

		out, err := e.runSSH(step.Host, b.String())
		if err != nil {
			return false, false, string(out), err
		}
		return true, false, strings.TrimSpace(string(out)), nil

	case "command":
		var b strings.Builder
		if r.Creates != "" {
			b.WriteString("if [ -e ")
			b.WriteString(shellQuote(r.Creates))
			b.WriteString(" ]; then echo __MASTERCHEF_SKIP_CREATES__; exit 0; fi\n")
		}
		if r.Unless != "" {
			b.WriteString("if sh -lc ")
			b.WriteString(shellQuote(r.Unless))
			b.WriteString(" >/dev/null 2>&1; then echo __MASTERCHEF_SKIP_UNLESS__; exit 0; fi\n")
		}
		b.WriteString("sh -lc ")
		b.WriteString(shellQuote(r.Command))
		b.WriteString("\n")

		out, err := e.runSSH(step.Host, b.String())
		outText := strings.TrimSpace(string(out))
		if outText == "__MASTERCHEF_SKIP_CREATES__" || outText == "__MASTERCHEF_SKIP_UNLESS__" {
			return false, true, outText, nil
		}
		if err != nil {
			return false, false, outText, err
		}
		return true, false, outText, nil
	default:
		return false, false, "", fmt.Errorf("unsupported resource type %q for ssh transport", r.Type)
	}
}

func (e *Executor) runSSH(host config.Host, script string) ([]byte, error) {
	target := host.Address
	if target == "" {
		target = host.Name
	}
	if host.User != "" {
		target = host.User + "@" + target
	}
	args := make([]string, 0, 8)
	if host.Port > 0 {
		args = append(args, "-p", strconv.Itoa(host.Port))
	}
	args = append(args, target, "sh", "-lc", script)

	ctx, cancel := context.WithTimeout(context.Background(), e.stepTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("ssh apply failed: %w: %s", err, string(out))
	}
	return out, nil
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
