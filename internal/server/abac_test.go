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

func TestABACEndpoints(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	features := filepath.Join(tmp, "features.md")
	if err := os.WriteFile(cfg, []byte(`version: v0
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

	deny := []byte(`{"name":"deny-freeze","effect":"deny","subject":"sre:oncall","resource":"run","action":"apply","conditions":{"freeze_active":"true"},"priority":100}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/access/abac/policies", bytes.NewReader(deny))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create deny policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	allow := []byte(`{"name":"allow-prod","effect":"allow","subject":"sre:oncall","resource":"run","action":"apply","conditions":{"environment":"prod"},"priority":10}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/abac/policies", bytes.NewReader(allow))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create allow policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	checkDenied := []byte(`{"subject":"sre:oncall","resource":"run","action":"apply","context":{"environment":"prod","freeze_active":"true"}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/abac/check", bytes.NewReader(checkDenied))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected abac denied check: code=%d body=%s", rr.Code, rr.Body.String())
	}

	checkAllowed := []byte(`{"subject":"sre:oncall","resource":"run","action":"apply","context":{"environment":"prod","freeze_active":"false"}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/abac/check", bytes.NewReader(checkAllowed))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected abac allowed check: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
