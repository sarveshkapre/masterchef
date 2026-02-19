package planner

import (
	"testing"

	"github.com/masterchef/masterchef/internal/config"
)

func TestBuild_IncludesHandlers(t *testing.T) {
	cfg := &config.Config{
		Version:   "v0",
		Inventory: config.Inventory{Hosts: []config.Host{{Name: "localhost", Transport: "local"}}},
		Resources: []config.Resource{{ID: "cfg", Type: "file", Host: "localhost", Path: "/tmp/cfg", NotifyHandlers: []string{"restart"}}},
		Handlers:  []config.Resource{{ID: "restart", Type: "command", Host: "localhost", Command: "echo restart"}},
	}
	plan, err := Build(cfg)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected one regular step, got %d", len(plan.Steps))
	}
	if len(plan.Handlers) != 1 {
		t.Fatalf("expected one handler step, got %d", len(plan.Handlers))
	}
	h, ok := plan.Handlers["restart"]
	if !ok {
		t.Fatalf("expected restart handler in plan")
	}
	if h.Resource.Type != "command" || h.Resource.Command == "" {
		t.Fatalf("unexpected handler step %+v", h)
	}
}
