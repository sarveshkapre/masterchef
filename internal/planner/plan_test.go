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
