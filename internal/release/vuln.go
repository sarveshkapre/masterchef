package release

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Dependency struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

type Advisory struct {
	ID              string `json:"id"`
	Module          string `json:"module"`
	Severity        string `json:"severity"`
	AffectedVersion string `json:"affected_version,omitempty"`
	FixedVersion    string `json:"fixed_version,omitempty"`
	Summary         string `json:"summary,omitempty"`
}

type CVEPolicy struct {
	BlockedSeverities []string `json:"blocked_severities"`
	AllowIDs          []string `json:"allow_ids,omitempty"`
}

type CVEViolation struct {
	Advisory   Advisory   `json:"advisory"`
	Dependency Dependency `json:"dependency"`
	Reason     string     `json:"reason"`
}

type CVEReport struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Policy      CVEPolicy      `json:"policy"`
	Violations  []CVEViolation `json:"violations"`
	Pass        bool           `json:"pass"`
}

func ListGoDependencies(root string) ([]Dependency, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	cmd := exec.Command("go", "list", "-m", "-json", "all")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	type module struct {
		Path    string `json:"Path"`
		Version string `json:"Version"`
	}
	deps := make([]Dependency, 0)
	buf := strings.Builder{}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "}" {
			buf.WriteString(line)
			var m module
			if err := json.Unmarshal([]byte(buf.String()), &m); err == nil {
				if m.Path != "" && m.Version != "" {
					deps = append(deps, Dependency{Path: m.Path, Version: m.Version})
				}
			}
			buf.Reset()
			continue
		}
		if buf.Len() > 0 || strings.TrimSpace(line) == "{" {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	if len(deps) == 0 {
		return nil, errors.New("no dependencies discovered")
	}
	sort.Slice(deps, func(i, j int) bool { return deps[i].Path < deps[j].Path })
	return deps, nil
}

func LoadAdvisories(path string) ([]Advisory, error) {
	path = filepath.Clean(path)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var advisories []Advisory
	if err := json.Unmarshal(b, &advisories); err != nil {
		return nil, err
	}
	return advisories, nil
}

func EvaluateCVEPolicy(deps []Dependency, advisories []Advisory, policy CVEPolicy) CVEReport {
	blockedSev := map[string]struct{}{}
	for _, s := range policy.BlockedSeverities {
		blockedSev[strings.ToLower(strings.TrimSpace(s))] = struct{}{}
	}
	if len(blockedSev) == 0 {
		blockedSev["critical"] = struct{}{}
		blockedSev["high"] = struct{}{}
	}
	allowSet := map[string]struct{}{}
	for _, id := range policy.AllowIDs {
		allowSet[strings.ToUpper(strings.TrimSpace(id))] = struct{}{}
	}

	violations := make([]CVEViolation, 0)
	for _, adv := range advisories {
		if _, allowed := allowSet[strings.ToUpper(strings.TrimSpace(adv.ID))]; allowed {
			continue
		}
		sev := strings.ToLower(strings.TrimSpace(adv.Severity))
		if _, blocked := blockedSev[sev]; !blocked {
			continue
		}
		for _, dep := range deps {
			if dep.Path != adv.Module {
				continue
			}
			if adv.AffectedVersion != "" && dep.Version != adv.AffectedVersion {
				continue
			}
			reason := "blocked severity " + adv.Severity
			if adv.FixedVersion != "" {
				reason += "; upgrade to " + adv.FixedVersion
			}
			violations = append(violations, CVEViolation{Advisory: adv, Dependency: dep, Reason: reason})
		}
	}
	return CVEReport{
		GeneratedAt: time.Now().UTC(),
		Policy:      policy,
		Violations:  violations,
		Pass:        len(violations) == 0,
	}
}
