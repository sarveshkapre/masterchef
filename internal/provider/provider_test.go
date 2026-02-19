package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/masterchef/masterchef/internal/config"
)

func TestBuiltinRegistry_HasCoreProviders(t *testing.T) {
	r := NewBuiltinRegistry()
	for _, typ := range []string{"file", "command"} {
		if _, ok := r.Lookup(typ); !ok {
			t.Fatalf("expected provider type %q in registry", typ)
		}
	}
}

func TestConformance_FileHandlerIsIdempotent(t *testing.T) {
	r := NewBuiltinRegistry()
	h, _ := r.Lookup("file")
	sample := config.Resource{
		ID:      "f1",
		Type:    "file",
		Host:    "localhost",
		Path:    filepath.Join(t.TempDir(), "x.txt"),
		Content: "hello\n",
	}
	rep := CheckIdempotency(context.Background(), h, sample)
	if !rep.IdempotentPass {
		t.Fatalf("expected idempotent pass, got %+v", rep)
	}
}

func TestCommandHandler_OnlyIfGuardSkips(t *testing.T) {
	r := NewBuiltinRegistry()
	h, ok := r.Lookup("command")
	if !ok {
		t.Fatalf("expected command handler in registry")
	}
	res, err := h.Apply(context.Background(), config.Resource{
		ID:      "cmd-only-if",
		Type:    "command",
		Host:    "localhost",
		Command: "echo should-not-run",
		OnlyIf:  "exit 1",
	})
	if err != nil {
		t.Fatalf("unexpected apply error: %v", err)
	}
	if !res.Skipped || res.Message != "command skipped: only_if condition failed" {
		t.Fatalf("expected only_if skip result, got %+v", res)
	}
}

func TestCommandHandler_OnlyIfGuardAllowsExecution(t *testing.T) {
	r := NewBuiltinRegistry()
	h, ok := r.Lookup("command")
	if !ok {
		t.Fatalf("expected command handler in registry")
	}
	marker := filepath.Join(t.TempDir(), "ok.marker")
	res, err := h.Apply(context.Background(), config.Resource{
		ID:      "cmd-only-if-run",
		Type:    "command",
		Host:    "localhost",
		Command: "touch " + marker,
		OnlyIf:  "exit 0",
	})
	if err != nil {
		t.Fatalf("unexpected apply error: %v", err)
	}
	if res.Skipped {
		t.Fatalf("expected command execution, got skipped result %+v", res)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected command to create marker, got %v", err)
	}
}
