package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Attestation struct {
	GeneratedAt       time.Time         `json:"generated_at"`
	SourceCommit      string            `json:"source_commit"`
	SourceBranch      string            `json:"source_branch,omitempty"`
	SourceTag         string            `json:"source_tag,omitempty"`
	SourceRemote      string            `json:"source_remote,omitempty"`
	SourceDirty       bool              `json:"source_dirty"`
	GoVersion         string            `json:"go_version"`
	GOOS              string            `json:"goos"`
	GOARCH            string            `json:"goarch"`
	TestCommand       string            `json:"test_command,omitempty"`
	TestPassed        bool              `json:"test_passed"`
	TestOutput        string            `json:"test_output,omitempty"`
	TestOutputSHA256  string            `json:"test_output_sha256,omitempty"`
	BuildUser         string            `json:"build_user,omitempty"`
	BuildHost         string            `json:"build_host,omitempty"`
	BuildEnvironment  map[string]string `json:"build_environment,omitempty"`
	ProvenanceVersion string            `json:"provenance_version"`
}

func GenerateAttestation(root, testCommand string) (Attestation, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	if _, err := os.Stat(root); err != nil {
		return Attestation{}, fmt.Errorf("stat root %q: %w", root, err)
	}
	commit := runCommandBestEffort(root, "git", "rev-parse", "HEAD")
	branch := runCommandBestEffort(root, "git", "rev-parse", "--abbrev-ref", "HEAD")
	tag := runCommandBestEffort(root, "git", "describe", "--tags", "--exact-match")
	remote := runCommandBestEffort(root, "git", "config", "--get", "remote.origin.url")
	dirty := strings.TrimSpace(runCommandBestEffort(root, "git", "status", "--porcelain")) != ""
	host, _ := os.Hostname()

	att := Attestation{
		GeneratedAt:       time.Now().UTC(),
		SourceCommit:      strings.TrimSpace(commit),
		SourceBranch:      strings.TrimSpace(branch),
		SourceTag:         strings.TrimSpace(tag),
		SourceRemote:      strings.TrimSpace(remote),
		SourceDirty:       dirty,
		GoVersion:         runtime.Version(),
		GOOS:              runtime.GOOS,
		GOARCH:            runtime.GOARCH,
		BuildUser:         strings.TrimSpace(os.Getenv("USER")),
		BuildHost:         strings.TrimSpace(os.Getenv("HOSTNAME")),
		BuildEnvironment:  gatherBuildEnvironment(),
		ProvenanceVersion: "v2",
	}
	if att.BuildHost == "" {
		att.BuildHost = strings.TrimSpace(host)
	}

	testCommand = strings.TrimSpace(testCommand)
	if testCommand != "" {
		att.TestCommand = testCommand
		cmd := shellCommand(testCommand)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		att.TestOutput = string(out)
		sum := sha256.Sum256(out)
		att.TestOutputSHA256 = hex.EncodeToString(sum[:])
		att.TestPassed = err == nil
	}
	return att, nil
}

func SaveAttestation(path string, att Attestation) error {
	b, err := json.MarshalIndent(att, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func runCommandBestEffort(root string, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func shellCommand(raw string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", raw)
	}
	return exec.Command("sh", "-lc", raw)
}

func gatherBuildEnvironment() map[string]string {
	keys := []string{
		"CI",
		"GITHUB_ACTIONS",
		"GITHUB_RUN_ID",
		"GITHUB_RUN_ATTEMPT",
		"GITHUB_SHA",
		"GITHUB_REF",
		"BUILD_ID",
		"BUILD_NUMBER",
		"BUILD_URL",
		"RUNNER_OS",
		"RUNNER_ARCH",
		"HOSTNAME",
	}
	out := map[string]string{}
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
