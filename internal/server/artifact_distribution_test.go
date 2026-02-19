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

func TestArtifactDistributionEndpoints(t *testing.T) {
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

	policy := []byte(`{"environment":"prod","max_transfer_mbps":120,"cache_warm_threshold_mb":150,"prefer_regional_cache":true,"relay_on_constrained_links":true,"compression":"zstd"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/artifact-distribution/policies", bytes.NewReader(policy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set artifact distribution policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/artifact-distribution/plan", bytes.NewReader([]byte(`{"environment":"prod","artifact_id":"bundle-v1","artifact_size_mb":500,"available_bandwidth_mbps":40}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("artifact distribution plan failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/artifact-distribution/plan", bytes.NewReader([]byte(`{"environment":"prod","artifact_id":"bundle-v2","artifact_size_mb":50,"available_bandwidth_mbps":0}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected bandwidth defer conflict response: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
