package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCompositionIncludesImportsOverlays(t *testing.T) {
	tmp := t.TempDir()
	includePath := filepath.Join(tmp, "include.yaml")
	importPath := filepath.Join(tmp, "import.yaml")
	overlayPath := filepath.Join(tmp, "overlay.yaml")
	mainPath := filepath.Join(tmp, "main.yaml")

	if err := os.WriteFile(includePath, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: base
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "base.txt")+`
    content: "include"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(importPath, []byte(`version: v0
resources:
  - id: imported
    type: command
    host: localhost
    command: "echo imported"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlayPath, []byte(`version: v0
resources:
  - id: base
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "base.txt")+`
    content: "overlay"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte(`version: v0
includes:
  - include.yaml
imports:
  - import.yaml
overlays:
  - overlay.yaml
resources:
  - id: base
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "base.txt")+`
    content: "main"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(mainPath)
	if err != nil {
		t.Fatalf("load composed config: %v", err)
	}
	if len(cfg.Inventory.Hosts) != 1 {
		t.Fatalf("expected host from include, got %+v", cfg.Inventory.Hosts)
	}
	if len(cfg.Resources) != 2 {
		t.Fatalf("expected 2 resources after composition, got %+v", cfg.Resources)
	}
	var base Resource
	for _, res := range cfg.Resources {
		if res.ID == "base" {
			base = res
		}
	}
	if base.Content != "overlay" {
		t.Fatalf("expected overlay to win for base content, got %q", base.Content)
	}
}

func TestLoadCompositionCycleDetection(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.yaml")
	b := filepath.Join(tmp, "b.yaml")
	if err := os.WriteFile(a, []byte(`version: v0
includes: [b.yaml]
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: a
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "a.txt")+`
    content: "a"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(`version: v0
includes: [a.yaml]
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: b
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "b.txt")+`
    content: "b"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(a); err == nil {
		t.Fatalf("expected composition cycle detection error")
	}
}
