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

func TestPlanGraphQueryEndpoint(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "graph-query.yaml")
	features := filepath.Join(tmp, "features.md")
	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: base
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "base.txt")+`
    content: "base"
  - id: app
    type: file
    host: localhost
    depends_on: [base]
    path: `+filepath.Join(tmp, "app.txt")+`
    content: "app"
  - id: notify
    type: command
    host: localhost
    depends_on: [app]
    command: "echo ok"
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

	query := []byte(`{"config_path":"graph-query.yaml","resource_id":"app","direction":"both","depth":2}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/plans/graph/query", bytes.NewReader(query))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("plan graph query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"upstream":["base"]`) || !strings.Contains(rr.Body.String(), `"downstream":["notify"]`) {
		t.Fatalf("expected upstream/downstream results: %s", rr.Body.String())
	}
}
