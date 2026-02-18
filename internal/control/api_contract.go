package control

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type APIDeprecation struct {
	Endpoint           string `json:"endpoint"`
	AnnouncedVersion   string `json:"announced_version"`
	RemoveAfterVersion string `json:"remove_after_version"`
	Replacement        string `json:"replacement,omitempty"`
}

type APISpec struct {
	Version      string           `json:"version"`
	Endpoints    []string         `json:"endpoints"`
	Deprecations []APIDeprecation `json:"deprecations,omitempty"`
}

type APIDiffReport struct {
	BaselineVersion          string           `json:"baseline_version"`
	CurrentVersion           string           `json:"current_version"`
	Added                    []string         `json:"added"`
	Removed                  []string         `json:"removed"`
	Unchanged                []string         `json:"unchanged"`
	DeprecationsAdded        []APIDeprecation `json:"deprecations_added,omitempty"`
	DeprecationViolations    []string         `json:"deprecation_violations,omitempty"`
	BackwardCompatible       bool             `json:"backward_compatible"`
	ForwardCompatible        bool             `json:"forward_compatible"`
	DeprecationLifecyclePass bool             `json:"deprecation_lifecycle_pass"`
}

type UpgradeAdvice struct {
	Severity string `json:"severity"` // error|warn|info
	Endpoint string `json:"endpoint,omitempty"`
	Message  string `json:"message"`
	Action   string `json:"action,omitempty"`
}

func DiffAPISpec(baseline, current APISpec) APIDiffReport {
	baseSet := map[string]struct{}{}
	curSet := map[string]struct{}{}
	for _, e := range baseline.Endpoints {
		baseSet[e] = struct{}{}
	}
	for _, e := range current.Endpoints {
		curSet[e] = struct{}{}
	}

	added := make([]string, 0)
	removed := make([]string, 0)
	unchanged := make([]string, 0)

	for e := range curSet {
		if _, ok := baseSet[e]; ok {
			unchanged = append(unchanged, e)
		} else {
			added = append(added, e)
		}
	}
	for e := range baseSet {
		if _, ok := curSet[e]; !ok {
			removed = append(removed, e)
		}
	}
	deprecationsAdded := diffDeprecations(baseline.Deprecations, current.Deprecations)
	violations := validateDeprecationLifecycle(removed, baseline.Deprecations, current.Version)

	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(unchanged)
	sort.Strings(violations)

	return APIDiffReport{
		BaselineVersion:          baseline.Version,
		CurrentVersion:           current.Version,
		Added:                    added,
		Removed:                  removed,
		Unchanged:                unchanged,
		DeprecationsAdded:        deprecationsAdded,
		DeprecationViolations:    violations,
		BackwardCompatible:       len(removed) == 0,
		ForwardCompatible:        len(added) == 0,
		DeprecationLifecyclePass: len(violations) == 0,
	}
}

func diffDeprecations(baseline, current []APIDeprecation) []APIDeprecation {
	base := map[string]APIDeprecation{}
	for _, d := range baseline {
		if ep := strings.TrimSpace(d.Endpoint); ep != "" {
			base[ep] = d
		}
	}
	out := make([]APIDeprecation, 0)
	for _, d := range current {
		ep := strings.TrimSpace(d.Endpoint)
		if ep == "" {
			continue
		}
		if _, ok := base[ep]; !ok {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Endpoint < out[j].Endpoint })
	return out
}

func validateDeprecationLifecycle(removed []string, baselineDeps []APIDeprecation, currentVersion string) []string {
	if len(removed) == 0 {
		return nil
	}
	baseByEndpoint := map[string]APIDeprecation{}
	for _, dep := range baselineDeps {
		ep := strings.TrimSpace(dep.Endpoint)
		if ep == "" {
			continue
		}
		baseByEndpoint[ep] = dep
	}
	currentMajor := versionMajor(currentVersion)
	violations := make([]string, 0)
	for _, ep := range removed {
		dep, ok := baseByEndpoint[ep]
		if !ok {
			violations = append(violations, ep+": removed without prior deprecation")
			continue
		}
		removeAfter := versionMajor(dep.RemoveAfterVersion)
		if removeAfter > 0 && currentMajor > 0 && currentMajor < removeAfter {
			violations = append(violations, ep+": removed before declared remove_after_version "+dep.RemoveAfterVersion)
		}
	}
	return violations
}

func versionMajor(raw string) int {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0
	}
	raw = strings.TrimPrefix(raw, "v")
	part := raw
	if idx := strings.Index(raw, "."); idx >= 0 {
		part = raw[:idx]
	}
	n, err := strconv.Atoi(part)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func GenerateUpgradeAdvice(report APIDiffReport) []UpgradeAdvice {
	out := make([]UpgradeAdvice, 0)
	for _, v := range report.DeprecationViolations {
		out = append(out, UpgradeAdvice{
			Severity: "error",
			Message:  v,
			Action:   "keep endpoint available until deprecation window is satisfied or publish migration exception",
		})
	}
	for _, dep := range report.DeprecationsAdded {
		msg := fmt.Sprintf("%s deprecated in %s and scheduled for removal after %s", dep.Endpoint, dep.AnnouncedVersion, dep.RemoveAfterVersion)
		out = append(out, UpgradeAdvice{
			Severity: "warn",
			Endpoint: dep.Endpoint,
			Message:  msg,
			Action:   replacementAction(dep.Replacement),
		})
	}
	if len(report.Removed) > 0 && report.DeprecationLifecyclePass {
		for _, ep := range report.Removed {
			out = append(out, UpgradeAdvice{
				Severity: "info",
				Endpoint: ep,
				Message:  ep + " removed with compliant lifecycle window",
				Action:   "verify downstream clients migrated to supported alternatives",
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		oi := adviceOrder(out[i].Severity)
		oj := adviceOrder(out[j].Severity)
		if oi != oj {
			return oi < oj
		}
		if out[i].Endpoint != out[j].Endpoint {
			return out[i].Endpoint < out[j].Endpoint
		}
		return out[i].Message < out[j].Message
	})
	return out
}

func adviceOrder(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "error":
		return 0
	case "warn":
		return 1
	default:
		return 2
	}
}

func replacementAction(replacement string) string {
	replacement = strings.TrimSpace(replacement)
	if replacement == "" {
		return "migrate client usage before scheduled removal"
	}
	return "migrate usage to " + replacement
}
