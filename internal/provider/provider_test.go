package provider

import (
	"context"
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
