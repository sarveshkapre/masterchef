package control

import (
	"errors"
	"sort"
	"strings"
	"time"
)

type DeploymentPreflightDependency struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Critical    bool   `json:"critical"`
}

type DeploymentPreflightCheckInput struct {
	Dependency string `json:"dependency"`
	Healthy    bool   `json:"healthy"`
	LatencyMS  int    `json:"latency_ms,omitempty"`
	Details    string `json:"details,omitempty"`
}

type DeploymentPreflightValidateInput struct {
	Profile string                          `json:"profile,omitempty"`
	Checks  []DeploymentPreflightCheckInput `json:"checks"`
}

type DeploymentPreflightCheckResult struct {
	Dependency string `json:"dependency"`
	Pass       bool   `json:"pass"`
	Severity   string `json:"severity"` // critical|warning|info
	Reason     string `json:"reason,omitempty"`
}

type DeploymentPreflightValidateResult struct {
	Ready          bool                             `json:"ready"`
	Profile        string                           `json:"profile,omitempty"`
	Results        []DeploymentPreflightCheckResult `json:"results"`
	Missing        []string                         `json:"missing,omitempty"`
	BlockingIssues []string                         `json:"blocking_issues,omitempty"`
	Warnings       []string                         `json:"warnings,omitempty"`
	CheckedAt      time.Time                        `json:"checked_at"`
}

func BuiltInDeploymentPreflightDependencies() []DeploymentPreflightDependency {
	out := []DeploymentPreflightDependency{
		{Name: "database", Description: "Control-plane relational state backend reachability and health", Critical: true},
		{Name: "dns", Description: "Service discovery and control-plane DNS resolution", Critical: true},
		{Name: "network", Description: "Inter-service network connectivity and latency budget", Critical: true},
		{Name: "queue", Description: "Scheduler/job queue availability and backpressure headroom", Critical: true},
		{Name: "storage", Description: "Artifact/object storage read/write path and consistency", Critical: true},
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func EvaluateDeploymentPreflight(in DeploymentPreflightValidateInput) (DeploymentPreflightValidateResult, error) {
	if len(in.Checks) == 0 {
		return DeploymentPreflightValidateResult{}, errors.New("checks are required")
	}
	required := map[string]DeploymentPreflightDependency{}
	for _, dep := range BuiltInDeploymentPreflightDependencies() {
		required[dep.Name] = dep
	}
	seen := map[string]struct{}{}
	results := make([]DeploymentPreflightCheckResult, 0, len(in.Checks))
	blocking := make([]string, 0)
	warnings := make([]string, 0)

	for _, check := range in.Checks {
		name := strings.ToLower(strings.TrimSpace(check.Dependency))
		if name == "" {
			return DeploymentPreflightValidateResult{}, errors.New("check dependency is required")
		}
		dep, ok := required[name]
		if !ok {
			return DeploymentPreflightValidateResult{}, errors.New("unsupported dependency: " + name)
		}
		seen[name] = struct{}{}
		reason := strings.TrimSpace(check.Details)
		if reason == "" {
			reason = "dependency health reported by preflight probe"
		}
		pass := check.Healthy
		severity := "info"
		if !pass {
			severity = "critical"
			blocking = append(blocking, name+" dependency is unhealthy")
			reason = "dependency is unhealthy"
		} else {
			threshold := preflightLatencyThreshold(name)
			if threshold > 0 && check.LatencyMS > threshold {
				severity = "warning"
				warnings = append(warnings, name+" latency exceeds recommended threshold")
				reason = "latency above recommended threshold"
			}
		}
		if dep.Critical && !pass {
			severity = "critical"
		}
		results = append(results, DeploymentPreflightCheckResult{
			Dependency: name,
			Pass:       pass,
			Severity:   severity,
			Reason:     reason,
		})
	}

	missing := make([]string, 0)
	for name := range required {
		if _, ok := seen[name]; !ok {
			missing = append(missing, name)
			blocking = append(blocking, "missing required dependency check: "+name)
		}
	}
	sort.Strings(missing)
	sort.Slice(results, func(i, j int) bool { return results[i].Dependency < results[j].Dependency })

	return DeploymentPreflightValidateResult{
		Ready:          len(blocking) == 0,
		Profile:        strings.TrimSpace(in.Profile),
		Results:        results,
		Missing:        emptyIfNil(missing),
		BlockingIssues: emptyIfNil(blocking),
		Warnings:       emptyIfNil(warnings),
		CheckedAt:      time.Now().UTC(),
	}, nil
}

func preflightLatencyThreshold(dep string) int {
	switch dep {
	case "network":
		return 200
	case "dns":
		return 100
	case "database":
		return 250
	case "queue":
		return 200
	case "storage":
		return 300
	default:
		return 0
	}
}

func emptyIfNil[T any](in []T) []T {
	if len(in) == 0 {
		return nil
	}
	return in
}
