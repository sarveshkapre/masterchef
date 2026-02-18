package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/masterchef/masterchef/internal/checker"
	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/executor"
	"github.com/masterchef/masterchef/internal/features"
	"github.com/masterchef/masterchef/internal/planner"
	"github.com/masterchef/masterchef/internal/policy"
	"github.com/masterchef/masterchef/internal/server"
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
	case "check":
		return runCheck(args[1:])
	case "apply":
		return runApply(args[1:])
	case "serve":
		return runServe(args[1:])
	case "policy":
		return runPolicy(args[1:])
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
  check [-f masterchef.yaml] [-min-confidence 1.0]
  apply [-f masterchef.yaml]
  serve [-addr :8080]
  policy [keygen|sign|verify] ...
  features [matrix|summary|verify] [-f features.md]
`))
	return errors.New("invalid command")
}

type ExitError struct {
	Code int
	Msg  string
}

func (e ExitError) Error() string {
	return e.Msg
}

func (e ExitError) ExitCode() int {
	return e.Code
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
	summary := fs.Bool("summary", false, "print blast-radius summary")
	graph := fs.Bool("graph", false, "print DOT execution graph")
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
	if *summary {
		s := planner.AnalyzeBlastRadius(p)
		sb, _ := json.MarshalIndent(s, "", "  ")
		fmt.Println(string(sb))
	}
	if *graph {
		fmt.Println(planner.ToDOT(p))
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

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	path := fs.String("f", "masterchef.yaml", "config path")
	minConfidence := fs.Float64("min-confidence", 1.0, "minimum required simulation confidence [0.0-1.0]")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *minConfidence < 0 || *minConfidence > 1 {
		return fmt.Errorf("min-confidence must be between 0.0 and 1.0")
	}

	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	p, err := planner.Build(cfg)
	if err != nil {
		return err
	}

	report := checker.Run(p)
	b, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(b))

	if report.Confidence < *minConfidence {
		return ExitError{
			Code: 3,
			Msg:  fmt.Sprintf("simulation confidence %.3f below required %.3f", report.Confidence, *minConfidence),
		}
	}
	if report.ChangesNeeded > 0 {
		return ExitError{
			Code: 2,
			Msg:  fmt.Sprintf("changes required: %d resources would change", report.ChangesNeeded),
		}
	}
	return nil
}

func runApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	path := fs.String("f", "masterchef.yaml", "config path")
	autoApprove := fs.Bool("yes", false, "auto approve apply without prompt")
	nonInteractive := fs.Bool("non-interactive", false, "fail instead of prompting for approval")
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

	if err := requireApplyApproval(p, *autoApprove, *nonInteractive); err != nil {
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

func requireApplyApproval(p *planner.Plan, autoApprove, nonInteractive bool) error {
	if autoApprove {
		return nil
	}
	if nonInteractive || strings.ToLower(os.Getenv("CI")) == "true" {
		return ExitError{
			Code: 5,
			Msg:  "apply requires explicit approval in non-interactive mode; re-run with -yes",
		}
	}

	fi, err := os.Stdin.Stat()
	if err != nil {
		return fmt.Errorf("stdin stat: %w", err)
	}
	if fi.Mode()&os.ModeCharDevice == 0 {
		return ExitError{
			Code: 5,
			Msg:  "apply requires interactive approval; no TTY detected, re-run with -yes",
		}
	}

	fmt.Printf("Apply plan with %d steps? [y/N]: ", len(p.Steps))
	in := bufio.NewReader(os.Stdin)
	line, err := in.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read approval input: %w", err)
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	if answer != "y" && answer != "yes" {
		return ExitError{Code: 5, Msg: "apply canceled by user"}
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

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "bind address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	s := server.New(*addr, ".")
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe()
	}()
	fmt.Printf("server listening on %s\n", *addr)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		fmt.Printf("received signal %s, shutting down\n", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.Shutdown(ctx)
	case err := <-errCh:
		return err
	}
}

func runPolicy(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("policy subcommand required: keygen|sign|verify")
	}
	switch args[0] {
	case "keygen":
		fs := flag.NewFlagSet("policy keygen", flag.ContinueOnError)
		privPath := fs.String("out", "policy-private.key", "private key output path")
		pubPath := fs.String("pub", "policy-public.key", "public key output path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		pub, priv, err := policy.GenerateKeypair()
		if err != nil {
			return err
		}
		if err := policy.SavePrivateKey(*privPath, priv); err != nil {
			return err
		}
		if err := policy.SavePublicKey(*pubPath, pub); err != nil {
			return err
		}
		fmt.Printf("keys written: private=%s public=%s\n", *privPath, *pubPath)
		return nil

	case "sign":
		fs := flag.NewFlagSet("policy sign", flag.ContinueOnError)
		cfgPath := fs.String("f", "masterchef.yaml", "config path")
		keyPath := fs.String("key", "policy-private.key", "private key path")
		out := fs.String("o", "policy-bundle.json", "bundle output path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		priv, err := policy.LoadPrivateKey(*keyPath)
		if err != nil {
			return err
		}
		bundle, err := policy.Build(*cfgPath)
		if err != nil {
			return err
		}
		if err := bundle.Sign(priv); err != nil {
			return err
		}
		if err := policy.SaveBundle(*out, bundle); err != nil {
			return err
		}
		fmt.Printf("bundle signed: %s\n", *out)
		return nil

	case "verify":
		fs := flag.NewFlagSet("policy verify", flag.ContinueOnError)
		bundlePath := fs.String("bundle", "policy-bundle.json", "bundle path")
		pubPath := fs.String("pub", "policy-public.key", "public key path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		pub, err := policy.LoadPublicKey(*pubPath)
		if err != nil {
			return err
		}
		bundle, err := policy.LoadBundle(*bundlePath)
		if err != nil {
			return err
		}
		if err := bundle.Verify(pub); err != nil {
			return err
		}
		fmt.Printf("bundle verified: %s\n", *bundlePath)
		return nil
	default:
		return fmt.Errorf("unknown policy subcommand %q", args[0])
	}
}
