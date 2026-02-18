package release

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateAttestationGitAndDirtyState(t *testing.T) {
	tmp := t.TempDir()
	writeAndCommitRepoFile(t, tmp, "tracked.txt", "v1")

	att, err := GenerateAttestation(tmp, "")
	if err != nil {
		t.Fatalf("generate attestation failed: %v", err)
	}
	if strings.TrimSpace(att.SourceCommit) == "" {
		t.Fatalf("expected source commit to be populated")
	}
	if att.SourceDirty {
		t.Fatalf("expected clean repository")
	}

	if err := os.WriteFile(filepath.Join(tmp, "tracked.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("modify tracked file failed: %v", err)
	}
	attDirty, err := GenerateAttestation(tmp, "")
	if err != nil {
		t.Fatalf("generate dirty attestation failed: %v", err)
	}
	if !attDirty.SourceDirty {
		t.Fatalf("expected dirty repository to be detected")
	}
}

func TestGenerateAndSaveAttestationWithTestCommand(t *testing.T) {
	tmp := t.TempDir()
	writeAndCommitRepoFile(t, tmp, "tracked.txt", "v1")

	att, err := GenerateAttestation(tmp, "echo ok")
	if err != nil {
		t.Fatalf("generate attestation with test command failed: %v", err)
	}
	if !att.TestPassed {
		t.Fatalf("expected test command to pass, output=%q", att.TestOutput)
	}
	if att.TestCommand != "echo ok" {
		t.Fatalf("unexpected test command: %q", att.TestCommand)
	}

	outPath := filepath.Join(tmp, "out", "attestation.json")
	if err := SaveAttestation(outPath, att); err != nil {
		t.Fatalf("save attestation failed: %v", err)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read attestation file failed: %v", err)
	}
	var loaded Attestation
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatalf("unmarshal attestation failed: %v", err)
	}
	if strings.TrimSpace(loaded.SourceCommit) == "" {
		t.Fatalf("expected persisted source commit")
	}
}

func writeAndCommitRepoFile(t *testing.T, root, relPath, contents string) {
	t.Helper()
	mustRun(t, root, "git", "init")
	mustRun(t, root, "git", "config", "user.email", "masterchef-test@example.com")
	mustRun(t, root, "git", "config", "user.name", "Masterchef Test")
	full := filepath.Join(root, relPath)
	if err := os.WriteFile(full, []byte(contents+"\n"), 0o644); err != nil {
		t.Fatalf("write %s failed: %v", relPath, err)
	}
	mustRun(t, root, "git", "add", relPath)
	mustRun(t, root, "git", "commit", "-m", "initial")
}

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %s %s: %v\n%s", name, strings.Join(args, " "), err, string(out))
	}
}
