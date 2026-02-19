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

func TestPolicySimulationCoverageInventory(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "sim.yaml")
	features := filepath.Join(tmp, "features.md")
	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: risky-command
    type: command
    host: localhost
    command: "echo risky"
    rescue_command: "echo rescue"
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

	body := []byte(`{"config_path":"sim.yaml","minimum_confidence":0.1}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/policy/simulate", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("policy simulate failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	payload := rr.Body.String()
	if !strings.Contains(payload, `"unsupported_inventory"`) || !strings.Contains(payload, `"resource_id":"risky-command"`) {
		t.Fatalf("expected unsupported inventory for risky-command resource: %s", payload)
	}
	if !strings.Contains(payload, `"coverage_by_resource_type"`) || !strings.Contains(payload, `"command"`) {
		t.Fatalf("expected coverage_by_resource_type to include command: %s", payload)
	}
}
