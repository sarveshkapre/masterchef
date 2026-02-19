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

func TestGitOpsReconcileEndpoint(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "drift.yaml")
	targetFile := filepath.Join(tmp, "managed.txt")
	features := filepath.Join(tmp, "features.md")
	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: managed-file
    type: file
    host: localhost
    path: `+targetFile+`
    content: "desired\n"
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

	body := []byte(`{
  "branch":"main",
  "config_path":"drift.yaml",
  "auto_enqueue":true,
  "max_drift_items":5,
  "priority":"high"
}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/gitops/reconcile", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reconcile endpoint failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	resp := rr.Body.String()
	if !strings.Contains(resp, `"would_reconcile":true`) {
		t.Fatalf("expected reconcile decision true, body=%s", resp)
	}
	if !strings.Contains(resp, `"job_id"`) {
		t.Fatalf("expected enqueued job id in response, body=%s", resp)
	}
}
