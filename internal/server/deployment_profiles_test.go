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

func TestDeploymentProfilesEndpoints(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/v1/control/deployment-profiles", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list deployment profiles failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	body := []byte(`{"profile":"minimal-footprint","object_store_backend":"filesystem","queue_mode":"embedded"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/deployment-profiles/evaluate", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("evaluate deployment profile failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
