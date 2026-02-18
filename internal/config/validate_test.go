package config

import "testing"

func TestValidate_OK(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{ID: "f1", Type: "file", Host: "localhost", Path: "/tmp/x", Content: "ok"},
			{ID: "c1", Type: "command", Host: "localhost", DependsOn: []string{"f1"}, Command: "echo ok"},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestValidate_CycleRefFailsUnknownDependency(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{ID: "a", Type: "file", Host: "localhost", Path: "/tmp/a", DependsOn: []string{"missing"}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected validation error for missing dependency")
	}
}

func TestValidate_ExecutionPolicy(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Execution: Execution{
			Strategy:          "free",
			MaxFailPercentage: 25,
		},
		Resources: []Resource{
			{ID: "f1", Type: "file", Host: "localhost", Path: "/tmp/x"},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected valid execution policy, got %v", err)
	}
	cfg.Execution.Strategy = "invalid"
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected invalid strategy error")
	}
	cfg.Execution.Strategy = "linear"
	cfg.Execution.MaxFailPercentage = 200
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected max_fail_percentage validation error")
	}
}

func TestValidate_NormalizesResourceTags(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{ID: "f1", Type: "file", Host: "localhost", Path: "/tmp/x", Tags: []string{"Prod", "prod", " api ", ""}},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if len(cfg.Resources[0].Tags) != 2 || cfg.Resources[0].Tags[0] != "api" || cfg.Resources[0].Tags[1] != "prod" {
		t.Fatalf("expected normalized sorted deduped tags, got %#v", cfg.Resources[0].Tags)
	}
}
