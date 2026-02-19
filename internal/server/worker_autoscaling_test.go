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

func TestWorkerAutoscalingEndpoints(t *testing.T) {
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

	setPolicy := []byte(`{"enabled":true,"min_workers":2,"max_workers":50,"queue_depth_per_worker":10,"target_p95_latency_ms":1000,"scale_up_step":5,"scale_down_step":2}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/autoscaling/policy", bytes.NewReader(setPolicy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set autoscaling policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	recommend := []byte(`{"queue_depth":200,"current_workers":10,"p95_latency_ms":2000}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/autoscaling/recommend", bytes.NewReader(recommend))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("autoscaling recommend failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"delta":`) {
		t.Fatalf("expected scaling delta in response, body=%s", rr.Body.String())
	}
}
