package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ResourceMatrixExpansionAndWhen(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "matrix.yaml")
	if err := os.WriteFile(cfgPath, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: deploy-{{service}}-{{env}}
    type: command
    host: localhost
    when: env == prod
    matrix:
      env: [prod, staging]
      service: [api, worker]
    command: "echo {{service}} {{env}}"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("load matrix config failed: %v", err)
	}
	if len(cfg.Resources) != 2 {
		t.Fatalf("expected 2 resources after when+matrix expansion, got %+v", cfg.Resources)
	}
	if cfg.Resources[0].ID != "deploy-api-prod" || cfg.Resources[0].Command != "echo api prod" {
		t.Fatalf("unexpected first expanded resource %+v", cfg.Resources[0])
	}
	if cfg.Resources[1].ID != "deploy-worker-prod" || cfg.Resources[1].Command != "echo worker prod" {
		t.Fatalf("unexpected second expanded resource %+v", cfg.Resources[1])
	}
}

func TestLoad_ResourceMatrixAutoIDSuffix(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "matrix-id.yaml")
	if err := os.WriteFile(cfgPath, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: health-check
    type: command
    host: localhost
    matrix:
      os: [linux, windows]
    command: "echo ok {{os}}"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("load matrix id config failed: %v", err)
	}
	if len(cfg.Resources) != 2 {
		t.Fatalf("expected 2 expanded resources, got %+v", cfg.Resources)
	}
	if cfg.Resources[0].ID != "health-check-os-linux" || cfg.Resources[1].ID != "health-check-os-windows" {
		t.Fatalf("expected auto-suffixed ids, got %+v", cfg.Resources)
	}
}
