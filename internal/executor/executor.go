package executor

import (
	"context"
	"fmt"
	"time"

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
