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

func TestProviderSandboxEndpoints(t *testing.T) {
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

	upsertBody := []byte(`{
		"provider":"provider.custom",
		"runtime":"wasi",
		"capabilities":["exec","read_state"],
		"filesystem_scope":["/var/lib/masterchef/providers/custom"],
		"network_scope":["api.internal:443"]
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/providers/sandbox/profiles", bytes.NewReader(upsertBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upsert provider sandbox profile failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/providers/sandbox/profiles/provider.custom", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get provider sandbox profile failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	evalBody := []byte(`{
		"provider":"provider.custom",
		"untrusted":true,
		"required_capabilities":["exec"],
		"require_filesystem":true,
		"require_network":true
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/providers/sandbox/evaluate", bytes.NewReader(evalBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("evaluate provider sandbox profile failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
