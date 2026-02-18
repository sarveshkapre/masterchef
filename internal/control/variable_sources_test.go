package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestVariableSourceRegistryInlineEnvFileHTTP(t *testing.T) {
	baseDir := t.TempDir()
	reg := NewVariableSourceRegistry(baseDir)

	t.Setenv("MC_VAR_DB_HOST", "db-prod")
	t.Setenv("MC_VAR_DB_PORT", "5432")

	filePath := filepath.Join(baseDir, "vars.yaml")
	if err := os.WriteFile(filePath, []byte("region: us-east-1\nservice:\n  name: payments\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"feature_flags":{"newCheckout":true}}`))
	}))
	defer httpSrv.Close()

	layers, err := reg.ResolveLayers(context.Background(), []VariableSourceSpec{
		{
			Name: "inline",
			Type: "inline",
			Config: map[string]any{
				"data": map[string]any{"service": map[string]any{"replicas": 3}},
			},
		},
		{
			Name: "env",
			Type: "env",
			Config: map[string]any{
				"prefix": "MC_VAR_",
				"target": "runtime.env",
			},
		},
		{
			Name: "file",
			Type: "file",
			Config: map[string]any{
				"path": "vars.yaml",
			},
		},
		{
			Name: "http",
			Type: "http",
			Config: map[string]any{
				"url": httpSrv.URL,
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve layers failed: %v", err)
	}
	if len(layers) != 4 {
		t.Fatalf("expected four layers, got %d", len(layers))
	}
	if layers[1].Data["runtime"] == nil {
		t.Fatalf("expected env layer target wrapping, got %#v", layers[1].Data)
	}
	if layers[2].Data["region"] != "us-east-1" {
		t.Fatalf("expected file layer parsing, got %#v", layers[2].Data)
	}
	ff, ok := layers[3].Data["feature_flags"].(map[string]any)
	if !ok || ff["newCheckout"] != true {
		t.Fatalf("expected http layer parsing, got %#v", layers[3].Data)
	}
}
