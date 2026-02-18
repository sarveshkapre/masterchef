package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInventoryGroupsEndpoint(t *testing.T) {
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
	cfg := filepath.Join(tmp, "masterchef.yaml")
	body := `version: v0
inventory:
  hosts:
    - name: web-01
      transport: local
      roles: [app]
      labels: {team: payments}
      topology: {zone: us-east-1a}
    - name: db-01
      transport: local
      roles: [db]
      labels: {team: payments}
      topology: {zone: us-east-1b}
resources:
  - id: f1
    type: file
    host: web-01
    path: /tmp/x
`
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/inventory/groups", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("inventory groups failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	resp := rr.Body.String()
	if !strings.Contains(resp, `"by_role"`) || !strings.Contains(resp, `"app"`) {
		t.Fatalf("expected role group content: %s", resp)
	}
	if !strings.Contains(resp, `"by_label"`) || !strings.Contains(resp, `"team=payments"`) {
		t.Fatalf("expected label group content: %s", resp)
	}
	if !strings.Contains(resp, `"by_topology"`) || !strings.Contains(resp, `"zone=us-east-1a"`) {
		t.Fatalf("expected topology group content: %s", resp)
	}
}
