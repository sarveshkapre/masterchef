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

func TestTenantLimitEndpoints(t *testing.T) {
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

	policy := []byte(`{"tenant":"tenant-a","requests_per_minute":120,"max_concurrent_runs":10,"max_queue_share_percent":40,"burst":20}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/tenancy/policies", bytes.NewReader(policy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set tenant policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	okCheck := []byte(`{"tenant":"tenant-a","requested_runs":1,"current_runs":2,"queue_depth":100,"tenant_queued":20}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/tenancy/admit-check", bytes.NewReader(okCheck))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected allowed admit-check: code=%d body=%s", rr.Code, rr.Body.String())
	}

	badCheck := []byte(`{"tenant":"tenant-a","requested_runs":1,"current_runs":2,"queue_depth":100,"tenant_queued":70}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/tenancy/admit-check", bytes.NewReader(badCheck))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected noisy-neighbor rejection: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
