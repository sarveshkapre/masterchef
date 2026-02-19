package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitOpsEnvironmentMaterializationEndpoints(t *testing.T) {
	tmp := t.TempDir()
	sourceCfg := filepath.Join(tmp, "base.yaml")
	features := filepath.Join(tmp, "features.md")
	if err := os.WriteFile(sourceCfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: marker
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "marker.txt")+`
    content: "ok"
`), 0o644); err != nil {
		t.Fatal(err)
	}
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

	materializeBody := []byte(`{
  "name":"staging",
  "branch":"env/staging",
  "source_config_path":"base.yaml",
  "auto_enqueue":true,
  "priority":"high"
}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/gitops/environments/materialize", bytes.NewReader(materializeBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("materialize failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"name":"staging"`) || !strings.Contains(rr.Body.String(), `"last_job_id"`) {
		t.Fatalf("unexpected materialize response: %s", rr.Body.String())
	}

	materializedPath := filepath.Join(tmp, ".masterchef", "materialized", "staging.yaml")
	b, err := os.ReadFile(materializedPath)
	if err != nil {
		t.Fatalf("expected materialized config file: %v", err)
	}
	if !strings.Contains(string(b), "source branch: env/staging") {
		t.Fatalf("expected branch marker in materialized file: %s", string(b))
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/gitops/environments", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"staging"`) {
		t.Fatalf("list gitops environments failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/gitops/environments/staging", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"branch":"env/staging"`) {
		t.Fatalf("get gitops environment failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
