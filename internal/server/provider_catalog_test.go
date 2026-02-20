package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProviderCatalogEndpoints(t *testing.T) {
	tmp := t.TempDir()
	features := filepath.Join(tmp, "features.md")
	if err := os.WriteFile(features, []byte(`# Features
- foo
## Competitor Feature Traceability Matrix (Strict 1:1)
### Chef -> Masterchef
| ID | Chef Feature | Masterchef 1:1 Mapping |
|---|---|---|
| CHEF-1 | X | foo |
`), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/providers/catalog", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list provider catalog failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	validateBody := []byte(`{"provider_id":"kubernetes.core","required_capabilities":["apply_manifest"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/providers/catalog/validate", bytes.NewReader(validateBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("validate provider catalog failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	sideEffectBody := []byte(`{"provider_id":"cloud.aws","required_capabilities":["ec2_instance"],"denied_side_effects":["network"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/providers/catalog/validate", bytes.NewReader(sideEffectBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected side-effect policy conflict: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
