package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/masterchef/masterchef/internal/policy"
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
