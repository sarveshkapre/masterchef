package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/masterchef/masterchef/internal/state"
)

func TestFleetHealthEndpoint(t *testing.T) {
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
	st := state.New(tmp)
	now := time.Now().UTC()
	_ = st.SaveRun(state.RunRecord{
		ID:        "run-ok",
		StartedAt: now.Add(-1 * time.Hour),
		EndedAt:   now.Add(-59 * time.Minute),
		Status:    state.RunSucceeded,
		Results: []state.ResourceRun{
			{ResourceID: "a", Host: "host-a", Changed: true},
		},
	})
	_ = st.SaveRun(state.RunRecord{
		ID:        "run-fail",
		StartedAt: now.Add(-30 * time.Minute),
		EndedAt:   now.Add(-29 * time.Minute),
		Status:    state.RunFailed,
		Results: []state.ResourceRun{
			{ResourceID: "b", Host: "host-b", Changed: true},
		},
	})

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/fleet/health?hours=24&slo=99", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("fleet health failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"runs_total":2`) || !strings.Contains(body, `"runs_failed":1`) {
		t.Fatalf("expected run counters in fleet health: %s", body)
	}
	if !strings.Contains(body, `"error_budget"`) || !strings.Contains(body, `"top_failing_hosts"`) {
		t.Fatalf("expected error budget and top hosts in response: %s", body)
	}
}
