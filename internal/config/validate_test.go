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
