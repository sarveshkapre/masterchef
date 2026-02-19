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

func TestCostSchedulingEndpoints(t *testing.T) {
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

	policy := []byte(`{"environment":"prod","max_cost_per_run":100,"max_hourly_budget":500,"off_peak_cost_multiplier":0.5,"throttle_above_percent":80}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/cost-scheduling/policies", bytes.NewReader(policy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set cost scheduling policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/cost-scheduling/admit", bytes.NewReader([]byte(`{"environment":"prod","estimated_cost":25,"hourly_spend":100,"queue_depth":5}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected allowed cost scheduling decision: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/cost-scheduling/admit", bytes.NewReader([]byte(`{"environment":"prod","estimated_cost":180,"hourly_spend":100,"queue_depth":5}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected cost scheduling rejection: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
