package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRBACEndpoints(t *testing.T) {
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

	createRole := []byte(`{"name":"deployer","permissions":[{"resource":"run","action":"apply","scope":"prod"}]}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/access/rbac/roles", bytes.NewReader(createRole))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create role failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var role struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &role)

	createBinding := []byte(`{"subject":"sre:oncall","role_id":"` + role.ID + `","scope":"prod"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/rbac/bindings", bytes.NewReader(createBinding))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create binding failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	checkAllowed := []byte(`{"subject":"sre:oncall","resource":"run","action":"apply","scope":"prod/service-a"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/rbac/check", bytes.NewReader(checkAllowed))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected allowed rbac check: code=%d body=%s", rr.Code, rr.Body.String())
	}

	checkDenied := []byte(`{"subject":"sre:oncall","resource":"run","action":"apply","scope":"staging"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/rbac/check", bytes.NewReader(checkDenied))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected denied rbac check: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
