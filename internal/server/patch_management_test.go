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

func TestPatchManagementEndpoints(t *testing.T) {
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

	policyBody := []byte(`{"environment":"prod","window_start_hour_utc":1,"window_duration_hours":4,"max_parallel_hosts":2,"allowed_classifications":["security","critical"],"require_reboot_approval":true}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/execution/patch/policies", bytes.NewReader(policyBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create patch policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	planBody := []byte(`{"environment":"prod","hour_utc":2,"reboot_approved":true,"hosts":[{"id":"node-1","classification":"security","needs_reboot":true},{"id":"node-2","classification":"critical","needs_reboot":false}]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/patch/plan", bytes.NewReader(planBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch plan failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
