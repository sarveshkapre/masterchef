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

func TestPlanGraphEndpoint(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "graph.yaml")
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

	body := []byte(`{"config_path":"graph.yaml"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/plans/graph", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("plan graph failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	payload := rr.Body.String()
	if !strings.Contains(payload, `"node_count":2`) || !strings.Contains(payload, `"edge_count":1`) {
		t.Fatalf("expected node/edge counts in response: %s", payload)
	}
	if !strings.Contains(payload, `"dot":"digraph masterchef_plan`) {
		t.Fatalf("expected dot graph in response: %s", payload)
	}
	if !strings.Contains(payload, `"mermaid":"flowchart LR`) {
		t.Fatalf("expected mermaid graph in response: %s", payload)
	}
}
