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

func TestDeploymentPreflightEndpoints(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/v1/control/deployment/preflight/dependencies", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list preflight dependencies failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	body := []byte(`{
		"profile":"ha",
		"checks":[
			{"dependency":"network","healthy":true,"latency_ms":50},
			{"dependency":"dns","healthy":true,"latency_ms":10},
			{"dependency":"storage","healthy":true,"latency_ms":120},
			{"dependency":"database","healthy":true,"latency_ms":100},
			{"dependency":"queue","healthy":true,"latency_ms":40}
		]
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/deployment/preflight/validate", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("preflight validate failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
