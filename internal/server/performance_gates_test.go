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

func TestPerformanceGateEndpoints(t *testing.T) {
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

	policy := []byte(`{"max_p95_latency_ms":1000,"min_throughput_rps":200,"max_error_budget_burn":1.2,"min_sample_count":100}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/release/performance-gates/policy", bytes.NewReader(policy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set performance gate policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	pass := []byte(`{"component":"scheduler","p95_latency_ms":900,"throughput_rps":230,"error_budget_burn_rate":0.8,"sample_count":120}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/performance-gates/evaluate", bytes.NewReader(pass))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected pass evaluation 200: code=%d body=%s", rr.Code, rr.Body.String())
	}

	fail := []byte(`{"component":"scheduler","p95_latency_ms":1800,"throughput_rps":80,"error_budget_burn_rate":1.9,"sample_count":70}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/performance-gates/evaluate", bytes.NewReader(fail))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected failing evaluation conflict (409): code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/release/performance-gates/evaluations?limit=5", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list performance evaluations failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
