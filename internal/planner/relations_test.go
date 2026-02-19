package planner

import (
	"testing"

	"github.com/masterchef/masterchef/internal/config"
)

func TestBuild_RequireBeforeNotifySubscribeRelations(t *testing.T) {
	cfg := &config.Config{
		Version:   "v0",
		Inventory: config.Inventory{Hosts: []config.Host{{Name: "localhost", Transport: "local"}}},
		Resources: []config.Resource{
			{ID: "a", Type: "file", Host: "localhost", Path: "/tmp/a"},
			{ID: "b", Type: "command", Host: "localhost", Command: "echo b", Require: []string{"a"}},
			{ID: "c", Type: "command", Host: "localhost", Command: "echo c", Before: []string{"d"}},
			{ID: "d", Type: "command", Host: "localhost", Command: "echo d"},
			{ID: "e", Type: "command", Host: "localhost", Command: "echo e", Notify: []string{"f"}},
			{ID: "f", Type: "command", Host: "localhost", Command: "echo f", Subscribe: []string{"e"}},
		},
	}
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("validate config: %v", err)
	}
	p, err := Build(cfg)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	order := map[string]int{}
	for _, step := range p.Steps {
		order[step.Resource.ID] = step.Order
	}
	if !(order["a"] < order["b"]) {
		t.Fatalf("expected require relation a < b, got %+v", order)
	}
	if !(order["c"] < order["d"]) {
		t.Fatalf("expected before relation c < d, got %+v", order)
	}
	if !(order["e"] < order["f"]) {
		t.Fatalf("expected notify/subscribe relation e < f, got %+v", order)
	}
}
