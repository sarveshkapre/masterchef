package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/masterchef/masterchef/internal/policy"
	"github.com/masterchef/masterchef/internal/release"
)

func TestRunReleaseSBOMSignVerify(t *testing.T) {
	tmp := t.TempDir()
	artifact := filepath.Join(tmp, "artifact.txt")
	if err := os.WriteFile(artifact, []byte("artifact"), 0o644); err != nil {
		t.Fatal(err)
	}
	sbomPath := filepath.Join(tmp, "sbom.json")
	signedPath := filepath.Join(tmp, "signed-sbom.json")
	privPath := filepath.Join(tmp, "priv.key")
	pubPath := filepath.Join(tmp, "pub.key")

	pub, priv, err := policy.GenerateKeypair()
	if err != nil {
		t.Fatalf("keygen failed: %v", err)
	}
	if err := policy.SavePrivateKey(privPath, priv); err != nil {
		t.Fatalf("save private key failed: %v", err)
	}
	if err := policy.SavePublicKey(pubPath, pub); err != nil {
		t.Fatalf("save public key failed: %v", err)
	}

	if err := runRelease([]string{"sbom", "-root", tmp, "-files", "artifact.txt", "-o", sbomPath}); err != nil {
		t.Fatalf("release sbom failed: %v", err)
	}
	if err := runRelease([]string{"sign", "-sbom", sbomPath, "-key", privPath, "-o", signedPath}); err != nil {
		t.Fatalf("release sign failed: %v", err)
	}
	if err := runRelease([]string{"verify", "-signed", signedPath, "-pub", pubPath}); err != nil {
		t.Fatalf("release verify failed: %v", err)
	}
}

func TestRunReleaseCVECheck(t *testing.T) {
	tmp := t.TempDir()
	deps, err := release.ListGoDependencies(".")
	if err != nil {
		t.Fatalf("list dependencies failed: %v", err)
	}
	if len(deps) == 0 {
		t.Fatalf("expected at least one dependency")
	}
	advPath := filepath.Join(tmp, "advisories.json")
	advisories := []release.Advisory{
		{
			ID:              "CVE-2026-TEST-0001",
			Module:          deps[0].Path,
			Severity:        "high",
			AffectedVersion: deps[0].Version,
			FixedVersion:    deps[0].Version + ".1",
		},
	}
	b, _ := json.Marshal(advisories)
	if err := os.WriteFile(advPath, b, 0o644); err != nil {
		t.Fatalf("write advisories failed: %v", err)
	}

	err = runRelease([]string{"cve-check", "-root", ".", "-advisories", advPath, "-blocked-severities", "high"})
	if err == nil {
		t.Fatalf("expected cve-check to block high severity advisory")
	}
	if ec, ok := err.(ExitError); !ok || ec.Code != 6 {
		t.Fatalf("expected ExitError code 6, got %v", err)
	}

	if err := runRelease([]string{"cve-check", "-root", ".", "-advisories", advPath, "-blocked-severities", "high", "-allow-ids", "CVE-2026-TEST-0001"}); err != nil {
		t.Fatalf("expected allow-id to pass cve-check, got %v", err)
	}
}

func TestRunReleaseAttest(t *testing.T) {
	tmp := t.TempDir()
	initGitRepo(t, tmp)
	attPath := filepath.Join(tmp, "attestation.json")

	if err := runRelease([]string{"attest", "-root", tmp, "-o", attPath, "-test-cmd", "echo ok"}); err != nil {
		t.Fatalf("release attest failed: %v", err)
	}
	raw, err := os.ReadFile(attPath)
	if err != nil {
		t.Fatalf("read attestation failed: %v", err)
	}
	var att release.Attestation
	if err := json.Unmarshal(raw, &att); err != nil {
		t.Fatalf("unmarshal attestation failed: %v", err)
	}
	if strings.TrimSpace(att.SourceCommit) == "" {
		t.Fatalf("expected source commit to be present")
	}
	if !att.TestPassed {
		t.Fatalf("expected test command to pass")
	}
}

func TestRunReleaseAttestFailsOnFailingTestCommand(t *testing.T) {
	tmp := t.TempDir()
	initGitRepo(t, tmp)
	attPath := filepath.Join(tmp, "attestation.json")

	err := runRelease([]string{"attest", "-root", tmp, "-o", attPath, "-test-cmd", "exit 3"})
	if err == nil {
		t.Fatalf("expected release attest to fail with non-zero test command")
	}
	ec, ok := err.(ExitError)
	if !ok || ec.Code != 7 {
		t.Fatalf("expected ExitError code 7, got %v", err)
	}
	if _, statErr := os.Stat(attPath); statErr != nil {
		t.Fatalf("expected attestation file to be written on failure, stat err=%v", statErr)
	}
}

func initGitRepo(t *testing.T, root string) {
	t.Helper()
	mustRun(t, root, "git", "init")
	mustRun(t, root, "git", "config", "user.email", "masterchef-test@example.com")
	mustRun(t, root, "git", "config", "user.name", "Masterchef Test")
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write tracked file failed: %v", err)
	}
	mustRun(t, root, "git", "add", "tracked.txt")
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
