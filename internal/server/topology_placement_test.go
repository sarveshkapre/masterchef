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

func TestTopologyPlacementEndpoints(t *testing.T) {
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

	policyBody := []byte(`{"environment":"prod","region":"us-east-1","zone":"us-east-1a","cluster":"payments","failure_domain":"rack-a","max_parallel":15}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/topology-placement/policies", bytes.NewReader(policyBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create topology placement policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	decisionBody := []byte(`{"environment":"prod","region":"us-east-1","zone":"us-east-1a","cluster":"payments","failure_domain":"rack-a","run_key":"deploy-v1"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/topology-placement/decide", bytes.NewReader(decisionBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("topology placement decision failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
