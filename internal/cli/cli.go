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
	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/executor"
	"github.com/masterchef/masterchef/internal/features"
	"github.com/masterchef/masterchef/internal/planner"
	"github.com/masterchef/masterchef/internal/policy"
	"github.com/masterchef/masterchef/internal/release"
	"github.com/masterchef/masterchef/internal/server"
	"github.com/masterchef/masterchef/internal/state"
	"github.com/masterchef/masterchef/internal/testimpact"
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
	case "fmt":
		return runFmt(args[1:])
	case "doctor":
		return runDoctor(args[1:])
	case "test-impact":
		return runTestImpact(args[1:])
	case "release":
		return runRelease(args[1:])
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
  fmt [-f masterchef.yaml] [-o canonical.yaml] [-format yaml|json]
  doctor [-f masterchef.yaml] [-format json|human]
  test-impact [-changes file1,file2,...] [-format json|human]
  release [sbom|sign|verify|cve-check|attest|upgrade-assist] ...
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

func runFmt(args []string) error {
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	path := fs.String("f", "masterchef.yaml", "config path")
	out := fs.String("o", "", "output path (defaults to stdout)")
	format := fs.String("format", "", "output format: yaml|json (defaults to input extension)")
	writeInPlace := fs.Bool("w", false, "write output in-place to input file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}

	outFormat := strings.TrimSpace(*format)
	if outFormat == "" {
		ext := strings.ToLower(filepath.Ext(*path))
		if ext == ".json" {
			outFormat = "json"
		} else {
			outFormat = "yaml"
		}
	}
	body, err := config.MarshalCanonical(cfg, outFormat)
	if err != nil {
		return err
	}
	target := strings.TrimSpace(*out)
	if *writeInPlace {
		target = *path
	}
	if target == "" {
		fmt.Print(string(body))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, body, 0o644); err != nil {
		return err
	}
	fmt.Printf("formatted config written: %s\n", target)
	return nil
}

func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	path := fs.String("f", "masterchef.yaml", "config path")
	format := fs.String("format", "human", "output format: human|json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	diags := config.Analyze(cfg)
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		b, _ := json.MarshalIndent(diags, "", "  ")
		fmt.Println(string(b))
	default:
		if len(diags) == 0 {
			fmt.Println("doctor: no issues found")
			return nil
		}
		for _, d := range diags {
			fmt.Printf("- [%s] %s: %s\n", d.Severity, d.Code, d.Message)
		}
	}
	for _, d := range diags {
		if d.Severity == config.SeverityError {
			return ExitError{Code: 4, Msg: "doctor found blocking errors"}
		}
	}
	return nil
}

func runTestImpact(args []string) error {
	fs := flag.NewFlagSet("test-impact", flag.ContinueOnError)
	changes := fs.String("changes", "", "comma-separated changed file paths")
	format := fs.String("format", "human", "output format: human|json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	files := make([]string, 0)
	for _, raw := range strings.Split(*changes, ",") {
		raw = strings.TrimSpace(raw)
		if raw != "" {
			files = append(files, raw)
		}
	}
	report := testimpact.Analyze(files)
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		b, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(b))
	default:
		if report.FallbackToAll {
			fmt.Printf("fallback-to-safe-set: %s\n", report.Reason)
		}
		fmt.Println("impacted packages:")
		for _, pkg := range report.ImpactedPackages {
			fmt.Printf("- %s\n", pkg)
		}
	}
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
	format := fs.String("format", "json", "output format: json|human|patch")
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
	switch strings.ToLower(*format) {
	case "json":
		b, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(b))
	case "human":
		fmt.Printf("resources=%d changes=%d simulatable=%d non_simulatable=%d confidence=%.3f\n",
			report.TotalResources, report.ChangesNeeded, report.Simulatable, report.NonSimulatable, report.Confidence)
		for _, it := range report.Items {
			state := "ok"
			if it.WouldChange {
				state = "change"
			}
			if !it.Simulatable {
				state = "unknown"
			}
			fmt.Printf("- [%s] %s (%s on %s): %s\n", state, it.ResourceID, it.Type, it.Host, it.Reason)
		}
	case "patch":
		for _, it := range report.Items {
			if it.Diff != "" {
				fmt.Printf("# %s (%s)\n%s\n", it.ResourceID, it.Type, it.Diff)
			}
		}
	default:
		return fmt.Errorf("unsupported check output format %q", *format)
	}

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
	reportPath := fs.String("report", "", "write machine-readable run report json to path")
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
	if *reportPath != "" {
		if err := os.MkdirAll(filepath.Dir(*reportPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(*reportPath, b, 0o644); err != nil {
			return err
		}
		fmt.Printf("run report written: %s\n", *reportPath)
	}
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

func runRelease(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("release subcommand required: sbom|sign|verify|cve-check|attest|upgrade-assist")
	}
	switch args[0] {
	case "sbom":
		fs := flag.NewFlagSet("release sbom", flag.ContinueOnError)
		root := fs.String("root", ".", "root directory for relative artifact paths")
		filesCSV := fs.String("files", ".", "comma-separated files/directories")
		out := fs.String("o", "sbom.json", "output sbom json path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		files := make([]string, 0)
		for _, f := range strings.Split(*filesCSV, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				files = append(files, f)
			}
		}
		sbom, err := release.GenerateSBOM(*root, files)
		if err != nil {
			return err
		}
		if err := release.SaveSBOM(*out, sbom); err != nil {
			return err
		}
		fmt.Printf("sbom generated: %s (%d artifacts)\n", *out, len(sbom.Artifacts))
		return nil

	case "sign":
		fs := flag.NewFlagSet("release sign", flag.ContinueOnError)
		sbomPath := fs.String("sbom", "sbom.json", "sbom path")
		keyPath := fs.String("key", "policy-private.key", "private key path")
		out := fs.String("o", "signed-sbom.json", "signed sbom output path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		sbom, err := release.LoadSBOM(*sbomPath)
		if err != nil {
			return err
		}
		priv, err := policy.LoadPrivateKey(*keyPath)
		if err != nil {
			return err
		}
		signed, err := release.SignSBOM(sbom, priv)
		if err != nil {
			return err
		}
		b, err := json.MarshalIndent(signed, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*out, b, 0o644); err != nil {
			return err
		}
		fmt.Printf("signed sbom written: %s\n", *out)
		return nil

	case "verify":
		fs := flag.NewFlagSet("release verify", flag.ContinueOnError)
		signedPath := fs.String("signed", "signed-sbom.json", "signed sbom path")
		pubPath := fs.String("pub", "policy-public.key", "public key path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		b, err := os.ReadFile(*signedPath)
		if err != nil {
			return err
		}
		var signed release.SignedSBOM
		if err := json.Unmarshal(b, &signed); err != nil {
			return err
		}
		pub, err := policy.LoadPublicKey(*pubPath)
		if err != nil {
			return err
		}
		if err := release.VerifySignedSBOM(signed, pub); err != nil {
			return err
		}
		fmt.Printf("signed sbom verified: %s\n", *signedPath)
		return nil
	case "cve-check":
		fs := flag.NewFlagSet("release cve-check", flag.ContinueOnError)
		root := fs.String("root", ".", "module root path for dependency discovery")
		advisoriesPath := fs.String("advisories", "", "advisories json path")
		blocked := fs.String("blocked-severities", "critical,high", "comma-separated blocked severities")
		allowIDs := fs.String("allow-ids", "", "comma-separated advisory IDs to allow")
		format := fs.String("format", "human", "output format: human|json")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*advisoriesPath) == "" {
			return fmt.Errorf("advisories path is required")
		}
		deps, err := release.ListGoDependencies(*root)
		if err != nil {
			return err
		}
		advisories, err := release.LoadAdvisories(*advisoriesPath)
		if err != nil {
			return err
		}
		policy := release.CVEPolicy{
			BlockedSeverities: splitCSV(*blocked),
			AllowIDs:          splitCSV(*allowIDs),
		}
		report := release.EvaluateCVEPolicy(deps, advisories, policy)
		if strings.EqualFold(strings.TrimSpace(*format), "json") {
			b, _ := json.MarshalIndent(report, "", "  ")
			fmt.Println(string(b))
		} else {
			if report.Pass {
				fmt.Println("cve-check: pass")
			} else {
				fmt.Printf("cve-check: %d blocking vulnerabilities found\n", len(report.Violations))
				for _, v := range report.Violations {
					fmt.Printf("- %s %s in %s@%s (%s)\n", v.Advisory.ID, v.Advisory.Severity, v.Dependency.Path, v.Dependency.Version, v.Reason)
				}
			}
		}
		if !report.Pass {
			return ExitError{Code: 6, Msg: "cve policy violations found"}
		}
		return nil
	case "attest":
		fs := flag.NewFlagSet("release attest", flag.ContinueOnError)
		root := fs.String("root", ".", "repository root path used for source metadata")
		out := fs.String("o", "attestation.json", "output attestation json path")
		testCmd := fs.String("test-cmd", "", "optional test command to execute and capture in attestation")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		att, err := release.GenerateAttestation(*root, *testCmd)
		if err != nil {
			return err
		}
		if err := release.SaveAttestation(*out, att); err != nil {
			return err
		}
		fmt.Printf("release attestation written: %s\n", *out)
		if att.TestCommand != "" && !att.TestPassed {
			return ExitError{Code: 7, Msg: "release attestation test command failed"}
		}
		return nil
	case "upgrade-assist":
		fs := flag.NewFlagSet("release upgrade-assist", flag.ContinueOnError)
		baselinePath := fs.String("baseline", "", "baseline API spec json path")
		currentPath := fs.String("current", "", "current API spec json path")
		format := fs.String("format", "human", "output format: human|json")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*baselinePath) == "" || strings.TrimSpace(*currentPath) == "" {
			return fmt.Errorf("both -baseline and -current are required")
		}
		baselineSpec, err := loadAPISpec(*baselinePath)
		if err != nil {
			return err
		}
		currentSpec, err := loadAPISpec(*currentPath)
		if err != nil {
			return err
		}
		report := control.DiffAPISpec(baselineSpec, currentSpec)
		advice := control.GenerateUpgradeAdvice(report)
		if strings.EqualFold(strings.TrimSpace(*format), "json") {
			b, _ := json.MarshalIndent(map[string]any{
				"report": report,
				"advice": advice,
			}, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("baseline=%s current=%s backward_compatible=%t lifecycle_pass=%t\n",
				report.BaselineVersion, report.CurrentVersion, report.BackwardCompatible, report.DeprecationLifecyclePass)
			if len(advice) == 0 {
				fmt.Println("no upgrade guidance generated")
			}
			for _, a := range advice {
				if a.Endpoint != "" {
					fmt.Printf("- [%s] %s: %s\n", a.Severity, a.Endpoint, a.Message)
				} else {
					fmt.Printf("- [%s] %s\n", a.Severity, a.Message)
				}
				if a.Action != "" {
					fmt.Printf("  action: %s\n", a.Action)
				}
			}
		}
		if !report.DeprecationLifecyclePass {
			return ExitError{Code: 8, Msg: "upgrade assistant detected deprecation lifecycle violations"}
		}
		return nil
	default:
		return fmt.Errorf("unknown release subcommand %q", args[0])
	}
}

func loadAPISpec(path string) (control.APISpec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return control.APISpec{}, err
	}
	var spec control.APISpec
	if err := json.Unmarshal(b, &spec); err != nil {
		return control.APISpec{}, err
	}
	return spec, nil
}

func splitCSV(raw string) []string {
	items := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}
