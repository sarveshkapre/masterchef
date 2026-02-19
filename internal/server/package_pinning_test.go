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

func TestPackagePinningEndpoints(t *testing.T) {
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

	policyBody := []byte(`{"target_kind":"host","target":"node-1","package":"nginx","version":"1.25.4","held":true,"enforce_drift":true}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/packages/pinning/policies", bytes.NewReader(policyBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create package pin policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	decisionBody := []byte(`{"target_kind":"host","target":"node-1","package":"nginx","installed_version":"1.26.0","hold_applied":true}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/packages/pinning/evaluate", bytes.NewReader(decisionBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("evaluate package pin policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
