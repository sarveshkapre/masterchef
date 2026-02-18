package control

import (
	"fmt"

	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/executor"
	"github.com/masterchef/masterchef/internal/planner"
	"github.com/masterchef/masterchef/internal/state"
)

type Runner struct {
	baseDir string
}

func NewRunner(baseDir string) *Runner {
	return &Runner{baseDir: baseDir}
}

func (r *Runner) ApplyPath(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	p, err := planner.Build(cfg)
	if err != nil {
		return fmt.Errorf("build plan: %w", err)
	}

	ex := executor.New(r.baseDir)
	run, err := ex.Apply(p)
	if err != nil {
		return err
	}
	st := state.New(r.baseDir)
	if err := st.SaveRun(run); err != nil {
		return err
	}
	if run.Status != state.RunSucceeded {
		return fmt.Errorf("apply failed")
	}
	return nil
}
