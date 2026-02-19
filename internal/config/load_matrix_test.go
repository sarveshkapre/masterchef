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

func TestLoad_ResourceLoopExpansionAndWhen(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "loop.yaml")
	if err := os.WriteFile(cfgPath, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: migrate-{{item}}
    type: command
    host: localhost
    loop: [prod, staging]
    when: item == prod
    command: "echo {{item}}"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("load loop config failed: %v", err)
	}
	if len(cfg.Resources) != 1 {
		t.Fatalf("expected 1 resource after loop+when expansion, got %+v", cfg.Resources)
	}
	if cfg.Resources[0].ID != "migrate-prod" || cfg.Resources[0].Command != "echo prod" {
		t.Fatalf("unexpected expanded resource %+v", cfg.Resources[0])
	}
}

func TestLoad_ResourceLoopWithMatrixExpansion(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "loop-matrix.yaml")
	if err := os.WriteFile(cfgPath, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: deploy-{{service}}-{{env}}
    type: command
    host: localhost
    loop_var: env
    loop: [prod, staging]
    matrix:
      service: [api, worker]
    command: "echo {{service}}-{{env}}"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("load loop+matrix config failed: %v", err)
	}
	if len(cfg.Resources) != 4 {
		t.Fatalf("expected 4 expanded resources, got %+v", cfg.Resources)
	}
	got := map[string]string{}
	for _, resource := range cfg.Resources {
		got[resource.ID] = resource.Command
	}
	want := map[string]string{
		"deploy-api-prod":       "echo api-prod",
		"deploy-worker-prod":    "echo worker-prod",
		"deploy-api-staging":    "echo api-staging",
		"deploy-worker-staging": "echo worker-staging",
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected expanded resource count: got=%d want=%d", len(got), len(want))
	}
	for id, command := range want {
		if got[id] != command {
			t.Fatalf("expected %s=%q, got %q", id, command, got[id])
		}
	}
}
