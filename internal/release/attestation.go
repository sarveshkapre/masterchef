package release

import (
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
	GeneratedAt  time.Time `json:"generated_at"`
	SourceCommit string    `json:"source_commit"`
	SourceDirty  bool      `json:"source_dirty"`
	GoVersion    string    `json:"go_version"`
	GOOS         string    `json:"goos"`
	GOARCH       string    `json:"goarch"`
	TestCommand  string    `json:"test_command,omitempty"`
	TestPassed   bool      `json:"test_passed"`
	TestOutput   string    `json:"test_output,omitempty"`
	BuildUser    string    `json:"build_user,omitempty"`
	BuildHost    string    `json:"build_host,omitempty"`
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
	dirty := strings.TrimSpace(runCommandBestEffort(root, "git", "status", "--porcelain")) != ""
	host, _ := os.Hostname()

	att := Attestation{
		GeneratedAt:  time.Now().UTC(),
		SourceCommit: strings.TrimSpace(commit),
		SourceDirty:  dirty,
		GoVersion:    runtime.Version(),
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
		BuildUser:    strings.TrimSpace(os.Getenv("USER")),
		BuildHost:    strings.TrimSpace(os.Getenv("HOSTNAME")),
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
