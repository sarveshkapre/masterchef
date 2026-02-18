package policy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBundleSignVerify(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	if err := os.WriteFile(cfg, []byte("version: v0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pub, priv, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	b, err := Build(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Sign(priv); err != nil {
		t.Fatal(err)
	}
	if err := b.Verify(pub); err != nil {
		t.Fatalf("expected verify success, got %v", err)
	}
}

func TestSaveLoadKeysAndBundle(t *testing.T) {
	tmp := t.TempDir()
	privPath := filepath.Join(tmp, "priv.key")
	pubPath := filepath.Join(tmp, "pub.key")
	bundlePath := filepath.Join(tmp, "bundle.json")

	pub, priv, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if err := SavePrivateKey(privPath, priv); err != nil {
		t.Fatal(err)
	}
	if err := SavePublicKey(pubPath, pub); err != nil {
		t.Fatal(err)
	}
	gotPriv, err := LoadPrivateKey(privPath)
	if err != nil {
		t.Fatal(err)
	}
	gotPub, err := LoadPublicKey(pubPath)
	if err != nil {
		t.Fatal(err)
	}

	b := &Bundle{ConfigPath: "x", ConfigSHA: "y"}
	if err := b.Sign(gotPriv); err != nil {
		t.Fatal(err)
	}
	if err := SaveBundle(bundlePath, b); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Verify(gotPub); err != nil {
		t.Fatal(err)
	}
}
