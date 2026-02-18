package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/executor"
	"github.com/masterchef/masterchef/internal/features"
	"github.com/masterchef/masterchef/internal/planner"
	"github.com/masterchef/masterchef/internal/state"
)

func Run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "init":
		return runInit(args[1:])
	case "validate":
		return runValidate(args[1:])
	case "plan":
		return runPlan(args[1:])
	case "apply":
		return runApply(args[1:])
	case "features":
		return runFeatures(args[1:])
	default:
		return usage()
	}
}

func usage() error {
	_, _ = fmt.Fprintln(os.Stderr, strings.TrimSpace(`
masterchef commands:
  init [-f masterchef.yaml]
  validate [-f masterchef.yaml]
  plan [-f masterchef.yaml] [-o plan.json]
  apply [-f masterchef.yaml]
  features [matrix|summary|verify] [-f features.md]
`))
	return errors.New("invalid command")
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	out := fs.String("f", "masterchef.yaml", "config path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("refusing to overwrite existing file %q", *out)
	}
	sample := `version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: ensure-readme
    type: file
    host: localhost
    path: ./tmp/masterchef-demo.txt
    content: "hello from masterchef\n"
  - id: announce
    type: command
    host: localhost
    depends_on: [ensure-readme]
    command: "echo applied"
    creates: ./tmp/masterchef-command.done
`
	if err := os.WriteFile(*out, []byte(sample), 0o644); err != nil {
		return err
	}
	fmt.Printf("initialized %s\n", *out)
	return nil
}

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	path := fs.String("f", "masterchef.yaml", "config path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := config.Load(*path); err != nil {
		return err
	}
	fmt.Printf("config valid: %s\n", *path)
	return nil
}

func runPlan(args []string) error {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	path := fs.String("f", "masterchef.yaml", "config path")
	out := fs.String("o", "", "write plan json to path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	p, err := planner.Build(cfg)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if *out == "" {
		fmt.Println(string(b))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		return err
	}
	fmt.Printf("plan written: %s\n", *out)
	return nil
}

func runApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	path := fs.String("f", "masterchef.yaml", "config path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	p, err := planner.Build(cfg)
	if err != nil {
		return err
	}
	ex := executor.New(".")
	run, err := ex.Apply(p)
	if err != nil {
		return err
	}
	st := state.New(".")
	if err := st.SaveRun(run); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(run, "", "  ")
	fmt.Println(string(b))
	if run.Status != state.RunSucceeded {
		return fmt.Errorf("apply failed")
	}
	return nil
}

func runFeatures(args []string) error {
	sub := "summary"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("features", flag.ContinueOnError)
	path := fs.String("f", "features.md", "features doc path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	doc, err := features.Parse(*path)
	if err != nil {
		return err
	}

	switch sub {
	case "summary":
		countByComp := map[string]int{}
		for _, row := range doc.Matrix {
			countByComp[row.Competitor]++
		}
		fmt.Printf("feature bullets: %d\n", len(doc.Bullets))
		fmt.Printf("traceability rows: %d\n", len(doc.Matrix))
		fmt.Printf("chef=%d ansible=%d puppet=%d salt=%d\n",
			countByComp["Chef"], countByComp["Ansible"], countByComp["Puppet"], countByComp["Salt"])
	case "matrix":
		b, _ := json.MarshalIndent(doc.Matrix, "", "  ")
		fmt.Println(string(b))
	case "verify":
		report := features.Verify(doc)
		b, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(b))
		return report.Error()
	default:
		return fmt.Errorf("unknown features subcommand %q", sub)
	}
	return nil
}
