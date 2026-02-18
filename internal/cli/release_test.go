package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
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
