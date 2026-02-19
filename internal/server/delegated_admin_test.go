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

func TestDelegatedAdminEndpoints(t *testing.T) {
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

	grant := []byte(`{"tenant":"tenant-a","environment":"prod","principal":"sre:oncall","scopes":["runs.cancel","workflows.*"],"delegator":"platform-admin"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/delegated-admin/grants", bytes.NewReader(grant))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create delegated admin grant failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/delegated-admin/authorize", bytes.NewReader([]byte(`{"tenant":"tenant-a","environment":"prod","principal":"sre:oncall","action":"workflows.launch"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delegated authorize should allow wildcard action: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/delegated-admin/authorize", bytes.NewReader([]byte(`{"tenant":"tenant-a","environment":"prod","principal":"sre:oncall","action":"packages.publish"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("delegated authorize should deny ungranted action: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
