package planner

import (
	"testing"

	"github.com/masterchef/masterchef/internal/config"
)

func TestBuild_DeterministicOrder(t *testing.T) {
	cfg := &config.Config{
		Version: "v0",
		Inventory: config.Inventory{
			Hosts: []config.Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []config.Resource{
			{ID: "b", Type: "file", Host: "localhost", Path: "/tmp/b", DependsOn: []string{"a"}},
			{ID: "a", Type: "file", Host: "localhost", Path: "/tmp/a"},
			{ID: "c", Type: "file", Host: "localhost", Path: "/tmp/c", DependsOn: []string{"b"}},
		},
	}
	p, err := Build(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := len(p.Steps), 3; got != want {
		t.Fatalf("steps count mismatch: got=%d want=%d", got, want)
	}
	if p.Steps[0].Resource.ID != "a" || p.Steps[1].Resource.ID != "b" || p.Steps[2].Resource.ID != "c" {
		t.Fatalf("unexpected order: %#v", p.Steps)
	}
	if p.Steps[0].Host.Transport != "local" {
		t.Fatalf("expected host transport in plan step")
	}
}

func TestBuild_CycleFails(t *testing.T) {
	cfg := &config.Config{
		Version: "v0",
		Inventory: config.Inventory{
			Hosts: []config.Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []config.Resource{
			{ID: "a", Type: "file", Host: "localhost", Path: "/tmp/a", DependsOn: []string{"b"}},
			{ID: "b", Type: "file", Host: "localhost", Path: "/tmp/b", DependsOn: []string{"a"}},
		},
	}
	if _, err := Build(cfg); err == nil {
		t.Fatalf("expected cycle detection error")
	}
}

func TestBuild_DelegateToOverridesExecutionHost(t *testing.T) {
	cfg := &config.Config{
		Version: "v0",
		Inventory: config.Inventory{
			Hosts: []config.Host{
				{Name: "target", Transport: "local"},
				{Name: "delegate", Transport: "local"},
			},
		},
		Resources: []config.Resource{
			{
				ID:         "c1",
				Type:       "command",
				Host:       "target",
				DelegateTo: "delegate",
				Command:    "echo delegated",
			},
		},
	}
	p, err := Build(cfg)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if len(p.Steps) != 1 {
		t.Fatalf("expected one step, got %d", len(p.Steps))
	}
	if p.Steps[0].Host.Name != "delegate" {
		t.Fatalf("expected execution host delegate, got %s", p.Steps[0].Host.Name)
	}
	if p.Steps[0].Resource.Host != "target" {
		t.Fatalf("expected resource host target to remain unchanged, got %s", p.Steps[0].Resource.Host)
	}
}
