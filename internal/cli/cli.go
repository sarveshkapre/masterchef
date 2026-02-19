package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/masterchef/masterchef/internal/checker"
	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/executor"
	"github.com/masterchef/masterchef/internal/features"
	"github.com/masterchef/masterchef/internal/grpcapi"
	"github.com/masterchef/masterchef/internal/planner"
	"github.com/masterchef/masterchef/internal/policy"
	"github.com/masterchef/masterchef/internal/release"
	"github.com/masterchef/masterchef/internal/server"
	"github.com/masterchef/masterchef/internal/state"
	"github.com/masterchef/masterchef/internal/testimpact"
	"google.golang.org/grpc"
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
	case "deploy":
		return runDeploy(args[1:])
	case "observe":
		return runObserve(args[1:])
	case "drift":
		return runDrift(args[1:])
	case "tui":
		return runTUI(args[1:])
	case "serve":
		return runServe(args[1:])
	case "dev":
		return runDev(args[1:])
	case "policy":
		return runPolicy(args[1:])
	case "features":
		return runFeatures(args[1:])
	case "docs":
		return runDocs(args[1:])
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
  plan [-f masterchef.yaml] [-o plan.json] [-snapshot plan.snapshot.json] [-update-snapshot]
  check [-f masterchef.yaml] [-min-confidence 1.0]
  apply [-f masterchef.yaml]
  deploy [-f masterchef.yaml] -env staging -branch env/staging [-yes]
  observe [-base .] [-limit 100] [-format json|human]
  drift [-base .] [-hours 24] [-format json|human]
  tui [-base .] [-limit 20]
  serve [-addr :8080] [-grpc-addr :9090]
  dev [-state-dir .masterchef/dev] [-addr :8080] [-grpc-addr :9090] [-dry-run]
  policy [keygen|sign|verify] ...
  features [matrix|summary|verify] [-f features.md]
  docs [verify-examples] [-format human|json]
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
	snapshotPath := fs.String("snapshot", "", "plan snapshot path for baseline comparison")
	updateSnapshot := fs.Bool("update-snapshot", false, "write or overwrite snapshot with current plan")
	snapshotFormat := fs.String("snapshot-format", "human", "snapshot output format: human|json")
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
	if strings.TrimSpace(*snapshotPath) != "" {
		if *updateSnapshot {
			if err := planner.SaveSnapshot(*snapshotPath, p); err != nil {
				return err
			}
			fmt.Printf("plan snapshot updated: %s\n", *snapshotPath)
		} else {
			diff, err := planner.CompareSnapshot(*snapshotPath, p)
			if err != nil {
				return err
			}
			if strings.EqualFold(strings.TrimSpace(*snapshotFormat), "json") {
				b, _ := json.MarshalIndent(diff, "", "  ")
				fmt.Println(string(b))
			} else {
				if diff.Match {
					fmt.Println("plan snapshot check: match")
				} else {
					fmt.Println("plan snapshot check: mismatch")
					if len(diff.AddedSteps) > 0 {
						fmt.Printf("- added steps: %s\n", strings.Join(diff.AddedSteps, ", "))
					}
					if len(diff.RemovedSteps) > 0 {
						fmt.Printf("- removed steps: %s\n", strings.Join(diff.RemovedSteps, ", "))
					}
					if len(diff.ChangedSteps) > 0 {
						fmt.Printf("- changed steps: %s\n", strings.Join(diff.ChangedSteps, ", "))
					}
				}
				fmt.Printf("baseline_hash=%s current_hash=%s\n", diff.BaselineHash, diff.CurrentHash)
			}
			if !diff.Match {
				return ExitError{Code: 9, Msg: "plan snapshot regression detected"}
			}
		}
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
	hostsFilter := fs.String("hosts", "", "comma-separated host filter for targeted checks")
	resourcesFilter := fs.String("resources", "", "comma-separated resource-id filter for targeted checks")
	includeTags := fs.String("tags", "", "comma-separated include-tags filter for targeted checks")
	skipTags := fs.String("skip-tags", "", "comma-separated skip-tags filter for targeted checks")
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
	p = filterPlanBySelectors(p, planSelectors{
		Hosts:       parseCSVSet(*hostsFilter),
		Resources:   parseCSVSet(*resourcesFilter),
		IncludeTags: parseCSVSet(*includeTags),
		SkipTags:    parseCSVSet(*skipTags),
	})
	if len(p.Steps) == 0 {
		return ExitError{Code: 7, Msg: "no plan steps matched target filters"}
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
	hostsFilter := fs.String("hosts", "", "comma-separated host filter for targeted applies")
	resourcesFilter := fs.String("resources", "", "comma-separated resource-id filter for targeted applies")
	includeTags := fs.String("tags", "", "comma-separated include-tags filter for targeted applies")
	skipTags := fs.String("skip-tags", "", "comma-separated skip-tags filter for targeted applies")
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
	p = filterPlanBySelectors(p, planSelectors{
		Hosts:       parseCSVSet(*hostsFilter),
		Resources:   parseCSVSet(*resourcesFilter),
		IncludeTags: parseCSVSet(*includeTags),
		SkipTags:    parseCSVSet(*skipTags),
	})
	if len(p.Steps) == 0 {
		return ExitError{Code: 7, Msg: "no plan steps matched target filters"}
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

func runDeploy(args []string) error {
	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	path := fs.String("f", "masterchef.yaml", "config path")
	environment := fs.String("env", "", "deployment environment name")
	branch := fs.String("branch", "", "deployment branch reference")
	autoApprove := fs.Bool("yes", false, "auto approve deployment without prompt")
	nonInteractive := fs.Bool("non-interactive", false, "fail instead of prompting for approval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*environment) == "" || strings.TrimSpace(*branch) == "" {
		return fmt.Errorf("deploy requires -env and -branch")
	}

	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	p, err := planner.Build(cfg)
	if err != nil {
		return err
	}
	if len(p.Steps) == 0 {
		return ExitError{Code: 7, Msg: "no plan steps to deploy"}
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
	trigger := map[string]string{
		"source":      "cli",
		"environment": strings.ToLower(strings.TrimSpace(*environment)),
		"branch":      strings.TrimSpace(*branch),
		"config_path": *path,
	}
	tb, _ := json.Marshal(trigger)
	fmt.Printf("deployment_trigger=%s\n", string(tb))
	if run.Status != state.RunSucceeded {
		return fmt.Errorf("deploy failed")
	}
	return nil
}

func runObserve(args []string) error {
	fs := flag.NewFlagSet("observe", flag.ContinueOnError)
	baseDir := fs.String("base", ".", "base directory containing .masterchef state")
	limit := fs.Int("limit", 100, "maximum runs to inspect")
	format := fs.String("format", "human", "output format: json|human")
	if err := fs.Parse(args); err != nil {
		return err
	}
	runs, err := state.New(*baseDir).ListRuns(*limit)
	if err != nil {
		return err
	}
	type observeReport struct {
		RunCount         int             `json:"run_count"`
		SucceededRuns    int             `json:"succeeded_runs"`
		FailedRuns       int             `json:"failed_runs"`
		ChangedResources int             `json:"changed_resources"`
		SkippedResources int             `json:"skipped_resources"`
		LastRunID        string          `json:"last_run_id,omitempty"`
		LastRunStatus    state.RunStatus `json:"last_run_status,omitempty"`
		LastRunStartedAt time.Time       `json:"last_run_started_at,omitempty"`
		LastRunEndedAt   time.Time       `json:"last_run_ended_at,omitempty"`
		TopChangedHosts  []string        `json:"top_changed_hosts,omitempty"`
		TopChangedTypes  []string        `json:"top_changed_resource_types,omitempty"`
	}
	report := observeReport{RunCount: len(runs)}
	hostCounts := map[string]int{}
	typeCounts := map[string]int{}
	for _, run := range runs {
		if run.Status == state.RunSucceeded {
			report.SucceededRuns++
		} else if run.Status == state.RunFailed {
			report.FailedRuns++
		}
		for _, res := range run.Results {
			if res.Changed {
				report.ChangedResources++
				host := strings.TrimSpace(res.Host)
				if host == "" {
					host = "unknown-host"
				}
				hostCounts[host]++
				t := strings.TrimSpace(strings.ToLower(res.Type))
				if t == "" {
					t = "unknown-type"
				}
				typeCounts[t]++
			}
			if res.Skipped {
				report.SkippedResources++
			}
		}
	}
	if len(runs) > 0 {
		report.LastRunID = runs[0].ID
		report.LastRunStatus = runs[0].Status
		report.LastRunStartedAt = runs[0].StartedAt
		report.LastRunEndedAt = runs[0].EndedAt
	}
	report.TopChangedHosts = topCountKeys(hostCounts, 5)
	report.TopChangedTypes = topCountKeys(typeCounts, 5)

	if strings.EqualFold(strings.TrimSpace(*format), "json") {
		b, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	fmt.Printf("runs=%d succeeded=%d failed=%d changed_resources=%d skipped_resources=%d\n",
		report.RunCount, report.SucceededRuns, report.FailedRuns, report.ChangedResources, report.SkippedResources)
	if report.LastRunID != "" {
		fmt.Printf("last_run=%s status=%s started=%s ended=%s\n",
			report.LastRunID, report.LastRunStatus, report.LastRunStartedAt.Format(time.RFC3339), report.LastRunEndedAt.Format(time.RFC3339))
	}
	if len(report.TopChangedHosts) > 0 {
		fmt.Printf("top_changed_hosts=%s\n", strings.Join(report.TopChangedHosts, ","))
	}
	if len(report.TopChangedTypes) > 0 {
		fmt.Printf("top_changed_types=%s\n", strings.Join(report.TopChangedTypes, ","))
	}
	return nil
}

func runDrift(args []string) error {
	fs := flag.NewFlagSet("drift", flag.ContinueOnError)
	baseDir := fs.String("base", ".", "base directory containing .masterchef state")
	hours := fs.Int("hours", 24, "lookback window in hours")
	format := fs.String("format", "human", "output format: json|human")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hours <= 0 {
		*hours = 24
	}
	if *hours > 24*30 {
		*hours = 24 * 30
	}
	since := time.Now().UTC().Add(-time.Duration(*hours) * time.Hour)
	runs, err := state.New(*baseDir).ListRuns(5000)
	if err != nil {
		return err
	}

	host := map[string]*cliDriftTrend{}
	typ := map[string]*cliDriftTrend{}
	totalChanged := 0
	failedRuns := 0

	for _, run := range runs {
		ref := run.StartedAt
		if ref.IsZero() {
			ref = run.EndedAt
		}
		if ref.IsZero() || ref.Before(since) {
			continue
		}
		if run.Status == state.RunFailed {
			failedRuns++
		}
		for _, res := range run.Results {
			if !res.Changed {
				continue
			}
			totalChanged++
			hk := strings.TrimSpace(res.Host)
			if hk == "" {
				hk = "unknown-host"
			}
			if host[hk] == nil {
				host[hk] = &cliDriftTrend{Key: hk}
			}
			host[hk].Count++
			if ref.After(host[hk].LastSeen) {
				host[hk].LastSeen = ref
			}

			tk := strings.TrimSpace(strings.ToLower(res.Type))
			if tk == "" {
				tk = "unknown-type"
			}
			if typ[tk] == nil {
				typ[tk] = &cliDriftTrend{Key: tk}
			}
			typ[tk].Count++
			if ref.After(typ[tk].LastSeen) {
				typ[tk].LastSeen = ref
			}
		}
	}

	hostTrends := flattenDrift(host, 10)
	typeTrends := flattenDrift(typ, 10)
	report := map[string]any{
		"window_hours":            *hours,
		"since":                   since,
		"total_changed_resources": totalChanged,
		"failed_runs":             failedRuns,
		"host_trends":             hostTrends,
		"resource_type_trends":    typeTrends,
	}
	if strings.EqualFold(strings.TrimSpace(*format), "json") {
		b, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	fmt.Printf("drift window=%dh changed_resources=%d failed_runs=%d\n", *hours, totalChanged, failedRuns)
	if len(hostTrends) > 0 {
		fmt.Printf("top_host=%s count=%d\n", hostTrends[0].Key, hostTrends[0].Count)
	}
	if len(typeTrends) > 0 {
		fmt.Printf("top_resource_type=%s count=%d\n", typeTrends[0].Key, typeTrends[0].Count)
	}
	return nil
}

func runTUI(args []string) error {
	return runTUIWithIO(args, os.Stdin, os.Stdout)
}

func runTUIWithIO(args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	baseDir := fs.String("base", ".", "base directory containing .masterchef state")
	limit := fs.Int("limit", 20, "maximum runs to load in inspector")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *limit <= 0 {
		*limit = 20
	}

	loadRuns := func() ([]state.RunRecord, error) {
		return state.New(*baseDir).ListRuns(*limit)
	}
	runs, err := loadRuns()
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		_, _ = fmt.Fprintln(out, "tui: no runs found")
		return nil
	}

	reader := bufio.NewReader(in)
	for {
		_, _ = fmt.Fprintln(out, "Masterchef Run Inspector")
		for i, run := range runs {
			_, _ = fmt.Fprintf(out, "%d) %s [%s] %s\n", i+1, run.ID, run.Status, run.StartedAt.Format(time.RFC3339))
		}
		_, _ = fmt.Fprint(out, "Select run number, r to refresh, q to quit: ")
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
		choice := strings.ToLower(strings.TrimSpace(line))
		switch choice {
		case "q", "quit", "exit":
			return nil
		case "r", "refresh":
			runs, err = loadRuns()
			if err != nil {
				return err
			}
			if len(runs) == 0 {
				_, _ = fmt.Fprintln(out, "tui: no runs found")
				return nil
			}
			continue
		}

		index, parseErr := strconv.Atoi(choice)
		if parseErr != nil || index < 1 || index > len(runs) {
			_, _ = fmt.Fprintln(out, "invalid selection")
			continue
		}
		run := runs[index-1]
		_, _ = fmt.Fprintf(out, "\nRun %s (%s)\n", run.ID, run.Status)
		_, _ = fmt.Fprintf(out, "Started: %s\n", run.StartedAt.Format(time.RFC3339))
		_, _ = fmt.Fprintf(out, "Ended: %s\n", run.EndedAt.Format(time.RFC3339))
		for _, res := range run.Results {
			_, _ = fmt.Fprintf(out, "- %s host=%s type=%s changed=%t skipped=%t\n", res.ResourceID, res.Host, res.Type, res.Changed, res.Skipped)
			if strings.TrimSpace(res.Message) != "" {
				_, _ = fmt.Fprintf(out, "  message: %s\n", strings.TrimSpace(res.Message))
			}
		}
		_, _ = fmt.Fprintln(out)
	}
}

func topCountKeys(m map[string]int, limit int) []string {
	type kv struct {
		Key   string
		Count int
	}
	items := make([]kv, 0, len(m))
	for k, v := range m {
		items = append(items, kv{Key: k, Count: v})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Key < items[j].Key
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Key)
	}
	return out
}

type planSelectors struct {
	Hosts       map[string]struct{}
	Resources   map[string]struct{}
	IncludeTags map[string]struct{}
	SkipTags    map[string]struct{}
}

func filterPlanBySelectors(p *planner.Plan, selectors planSelectors) *planner.Plan {
	if p == nil {
		return &planner.Plan{}
	}
	steps := make([]planner.Step, 0, len(p.Steps))
	for _, step := range p.Steps {
		if !matchesSelectors(step, selectors) {
			continue
		}
		steps = append(steps, step)
	}
	return &planner.Plan{
		Execution: p.Execution,
		Steps:     steps,
	}
}

func matchesSelectors(step planner.Step, selectors planSelectors) bool {
	host := strings.TrimSpace(step.Resource.Host)
	if host == "" {
		host = strings.TrimSpace(step.Host.Name)
	}
	if len(selectors.Hosts) > 0 {
		if _, ok := selectors.Hosts[strings.ToLower(host)]; !ok {
			return false
		}
	}
	if len(selectors.Resources) > 0 {
		if _, ok := selectors.Resources[strings.ToLower(strings.TrimSpace(step.Resource.ID))]; !ok {
			return false
		}
	}
	tagSet := map[string]struct{}{}
	for _, tag := range step.Resource.Tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag != "" {
			tagSet[tag] = struct{}{}
		}
	}
	if len(selectors.IncludeTags) > 0 {
		matched := false
		for tag := range selectors.IncludeTags {
			if _, ok := tagSet[tag]; ok {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(selectors.SkipTags) > 0 {
		for tag := range selectors.SkipTags {
			if _, ok := tagSet[tag]; ok {
				return false
			}
		}
	}
	return true
}

func parseCSVSet(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		out[part] = struct{}{}
	}
	return out
}

type cliDriftTrend struct {
	Key      string    `json:"key"`
	Count    int       `json:"count"`
	LastSeen time.Time `json:"last_seen"`
}

func flattenDrift(in map[string]*cliDriftTrend, limit int) []cliDriftTrend {
	out := make([]cliDriftTrend, 0, len(in))
	for _, item := range in {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
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

func runDocs(args []string) error {
	sub := "verify-examples"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("docs", flag.ContinueOnError)
	format := fs.String("format", "human", "output format: human|json")
	if err := fs.Parse(args); err != nil {
		return err
	}

	switch sub {
	case "verify-examples":
		report := control.VerifyActionDocExamples(control.NewActionDocCatalog().List(), nil)
		if strings.EqualFold(strings.TrimSpace(*format), "json") {
			b, _ := json.MarshalIndent(report, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("checked=%d passed=%t\n", report.Checked, report.Passed)
			for _, item := range report.Failures {
				fmt.Printf("- %s\n", item)
			}
		}
		if !report.Passed {
			return ExitError{Code: 6, Msg: "documentation example verification failed"}
		}
		return nil
	default:
		return fmt.Errorf("unknown docs subcommand %q", sub)
	}
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "bind address")
	grpcAddr := fs.String("grpc-addr", "", "optional gRPC bind address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return serveRuntime(*addr, *grpcAddr, ".")
}

func runDev(args []string) error {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	stateDir := fs.String("state-dir", ".masterchef/dev", "local dev state directory")
	addr := fs.String("addr", ":8080", "http bind address")
	grpcAddr := fs.String("grpc-addr", ":9090", "grpc bind address")
	dryRun := fs.Bool("dry-run", false, "print computed local dev runtime and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root := strings.TrimSpace(*stateDir)
	if root == "" {
		return fmt.Errorf("state-dir is required")
	}
	root = filepath.Clean(root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	objectStorePath := filepath.Join(root, "objectstore")
	if err := os.Setenv("MC_OBJECT_STORE_BACKEND", "filesystem"); err != nil {
		return err
	}
	if err := os.Setenv("MC_OBJECT_STORE_PATH", objectStorePath); err != nil {
		return err
	}
	if *dryRun {
		fmt.Printf("dev runtime prepared: state_dir=%s object_store=%s addr=%s grpc_addr=%s\n", root, objectStorePath, *addr, *grpcAddr)
		return nil
	}
	fmt.Printf("dev mode active: state_dir=%s object_store=%s\n", root, objectStorePath)
	return serveRuntime(*addr, *grpcAddr, ".")
}

func serveRuntime(addr, grpcAddr, baseDir string) error {
	s := server.New(addr, baseDir)
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe()
	}()
	fmt.Printf("server listening on %s\n", addr)

	var grpcServer *grpc.Server
	var grpcLis net.Listener
	if strings.TrimSpace(grpcAddr) != "" {
		g, lis, err := grpcapi.Listen(grpcAddr, baseDir)
		if err != nil {
			return err
		}
		grpcServer = g
		grpcLis = lis
		go func() {
			errCh <- g.Serve(lis)
		}()
		fmt.Printf("grpc listening on %s\n", grpcAddr)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		fmt.Printf("received signal %s, shutting down\n", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if grpcServer != nil {
			done := make(chan struct{})
			go func() {
				grpcServer.GracefulStop()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				grpcServer.Stop()
			}
		}
		if grpcLis != nil {
			_ = grpcLis.Close()
		}
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
