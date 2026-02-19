package testimpact

import (
	"path/filepath"
	"sort"
	"strings"
)

type Report struct {
	ChangedFiles     []string `json:"changed_files"`
	ImpactedPackages []string `json:"impacted_packages"`
	FallbackToAll    bool     `json:"fallback_to_all"`
	Reason           string   `json:"reason,omitempty"`
	Scope            string   `json:"scope"`
	RecommendedTest  string   `json:"recommended_test"`
}

type AnalyzeOptions struct {
	AlwaysInclude      []string
	MaxTargetedPackage int
}

var criticalFallbackPackages = []string{
	"./internal/config",
	"./internal/planner",
	"./internal/executor",
	"./internal/server",
	"./internal/control",
}

func Analyze(changedFiles []string) Report {
	return AnalyzeWithOptions(changedFiles, AnalyzeOptions{})
}

func AnalyzeWithOptions(changedFiles []string, opts AnalyzeOptions) Report {
	pkgSet := map[string]struct{}{}
	fallback := false
	reason := ""

	cleaned := make([]string, 0, len(changedFiles))
	for _, f := range changedFiles {
		f = normalizePath(f)
		if f == "" {
			continue
		}
		cleaned = append(cleaned, f)
		pkg := packageForPath(f)
		if pkg == "" {
			fallback = true
			reason = "found unknown path mapping"
			continue
		}
		pkgSet[pkg] = struct{}{}
	}

	if len(cleaned) == 0 {
		fallback = true
		reason = "no changed files provided"
	}

	for _, pkg := range opts.AlwaysInclude {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			continue
		}
		pkgSet[pkg] = struct{}{}
	}

	for pkg := range pkgSet {
		if pkg == "./internal/config" || pkg == "./internal/planner" || pkg == "./internal/executor" || pkg == "./internal/server" || pkg == "./internal/control" {
			pkgSet["./internal/cli"] = struct{}{}
		}
	}

	if fallback {
		for _, pkg := range criticalFallbackPackages {
			pkgSet[pkg] = struct{}{}
		}
		pkgSet["./internal/cli"] = struct{}{}
	}

	impacted := make([]string, 0, len(pkgSet))
	for pkg := range pkgSet {
		impacted = append(impacted, pkg)
	}
	sort.Strings(impacted)
	if !fallback && opts.MaxTargetedPackage > 0 && len(impacted) > opts.MaxTargetedPackage {
		fallback = true
		reason = "impacted package set exceeds max targeted threshold"
		for _, pkg := range criticalFallbackPackages {
			pkgSet[pkg] = struct{}{}
		}
		pkgSet["./internal/cli"] = struct{}{}
		impacted = impacted[:0]
		for pkg := range pkgSet {
			impacted = append(impacted, pkg)
		}
		sort.Strings(impacted)
	}
	scope := "targeted"
	if fallback {
		scope = "safe-fallback"
	}
	return Report{
		ChangedFiles:     cleaned,
		ImpactedPackages: impacted,
		FallbackToAll:    fallback,
		Reason:           reason,
		Scope:            scope,
		RecommendedTest:  "go test " + strings.Join(impacted, " "),
	}
}

func packageForPath(path string) string {
	path = normalizePath(path)
	switch {
	case strings.HasPrefix(path, "internal/config/"):
		return "./internal/config"
	case strings.HasPrefix(path, "internal/planner/"):
		return "./internal/planner"
	case strings.HasPrefix(path, "internal/executor/"):
		return "./internal/executor"
	case strings.HasPrefix(path, "internal/server/"):
		return "./internal/server"
	case strings.HasPrefix(path, "internal/control/"):
		return "./internal/control"
	case strings.HasPrefix(path, "internal/state/"):
		return "./internal/state"
	case strings.HasPrefix(path, "internal/provider/"):
		return "./internal/provider"
	case strings.HasPrefix(path, "internal/features/"):
		return "./internal/features"
	case strings.HasPrefix(path, "internal/checker/"):
		return "./internal/checker"
	case strings.HasPrefix(path, "internal/cli/"):
		return "./internal/cli"
	case strings.HasPrefix(path, "internal/storage/"):
		return "./internal/storage"
	case strings.HasPrefix(path, "cmd/masterchef/"):
		return "./cmd/masterchef"
	case strings.HasPrefix(path, "go.mod"), strings.HasPrefix(path, "go.sum"):
		return "./..."
	default:
		return ""
	}
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimPrefix(path, "/")
	return path
}
