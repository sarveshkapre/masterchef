package release

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type ToolchainReport struct {
	GoDirective        string   `json:"go_directive,omitempty"`
	ToolchainDirective string   `json:"toolchain_directive,omitempty"`
	RuntimeGoVersion   string   `json:"runtime_go_version"`
	Pinned             bool     `json:"pinned"`
	Match              bool     `json:"match"`
	Reason             string   `json:"reason,omitempty"`
	SuggestedPipeline  []string `json:"suggested_pipeline,omitempty"`
}

func CheckToolchain(root string) (ToolchainReport, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	modPath := filepath.Join(root, "go.mod")
	f, err := os.Open(modPath)
	if err != nil {
		return ToolchainReport{}, err
	}
	defer f.Close()

	var goDirective string
	var toolchainDirective string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "go ") && goDirective == "" {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				goDirective = normalizeGoVersion(parts[1])
			}
			continue
		}
		if strings.HasPrefix(line, "toolchain ") && toolchainDirective == "" {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				toolchainDirective = normalizeGoVersion(parts[1])
			}
		}
	}
	if err := sc.Err(); err != nil {
		return ToolchainReport{}, err
	}
	if goDirective == "" && toolchainDirective == "" {
		return ToolchainReport{}, errors.New("go.mod missing go/toolchain directives")
	}

	runtimeVer := normalizeGoVersion(runtime.Version())
	report := ToolchainReport{
		GoDirective:        goDirective,
		ToolchainDirective: toolchainDirective,
		RuntimeGoVersion:   runtimeVer,
		Pinned:             toolchainDirective != "",
		SuggestedPipeline: []string{
			"GOWORK=off GOFLAGS=-mod=readonly go test ./...",
			"GOWORK=off GOFLAGS=-mod=readonly go run ./cmd/masterchef features verify -f features.md",
		},
	}
	if report.Pinned {
		report.Match = runtimeVer == toolchainDirective
		if report.Match {
			report.Reason = "runtime matches pinned toolchain directive"
		} else {
			report.Reason = "runtime must match toolchain directive exactly"
		}
		return report, nil
	}

	runtimeMajorMinor := majorMinor(runtimeVer)
	goMajorMinor := majorMinor(goDirective)
	report.Match = runtimeMajorMinor == goMajorMinor
	if report.Match {
		report.Reason = "runtime matches go directive major.minor"
	} else {
		report.Reason = "runtime must match go directive major.minor for reproducibility"
	}
	return report, nil
}

func normalizeGoVersion(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.TrimPrefix(raw, "go")
	return raw
}

func majorMinor(raw string) string {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return raw
}
