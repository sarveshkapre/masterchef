package release

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateAndSignSBOM(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "a.txt")
	if err := os.WriteFile(fileA, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	sbom, err := GenerateSBOM(tmp, []string{"a.txt"})
	if err != nil {
		t.Fatalf("generate sbom failed: %v", err)
	}
	if len(sbom.Artifacts) != 1 {
		t.Fatalf("expected one artifact")
	}
	if err := SaveSBOM(filepath.Join(tmp, "sbom.json"), sbom); err != nil {
		t.Fatalf("save sbom failed: %v", err)
	}
	loaded, err := LoadSBOM(filepath.Join(tmp, "sbom.json"))
	if err != nil {
		t.Fatalf("load sbom failed: %v", err)
	}
	if len(loaded.Artifacts) != 1 {
		t.Fatalf("expected one loaded artifact")
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen failed: %v", err)
	}
	signed, err := SignSBOM(sbom, priv)
	if err != nil {
		t.Fatalf("sign sbom failed: %v", err)
	}
	if err := VerifySignedSBOM(signed, pub); err != nil {
		t.Fatalf("verify sbom failed: %v", err)
	}
}
