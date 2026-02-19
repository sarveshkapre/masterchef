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

func TestMultiMasterEndpoints(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "mm.yaml")
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

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/multi-master/nodes", bytes.NewReader([]byte(`{"node_id":"cp-us-1","region":"us-east-1","address":"10.0.0.1","role":"primary","status":"active"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"node_id":"cp-us-1"`) {
		t.Fatalf("create multi-master node failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/multi-master/nodes/cp-us-1/heartbeat", bytes.NewReader([]byte(`{"status":"degraded"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"status":"degraded"`) {
		t.Fatalf("heartbeat failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewReader([]byte(`{"config_path":"mm.yaml"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("enqueue job failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader([]byte(`{"type":"run.started","message":"test"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("ingest event failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/multi-master/cache", bytes.NewReader([]byte(`{"event_limit":50,"max_entries":500}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"synced_jobs"`) {
		t.Fatalf("sync multi-master cache failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/multi-master/cache?kind=job&limit=10", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list cache failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"kind":"job"`) {
		t.Fatalf("expected job cache entries: %s", rr.Body.String())
	}
}
