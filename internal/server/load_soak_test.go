package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSoakEndpoints(t *testing.T) {
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

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/release/tests/load-soak/suites", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list load-soak suites failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	createSuite := []byte(`{
		"id":"soak-workers-nightly",
		"name":"Workers Nightly Soak",
		"target_component":"execution-workers",
		"mode":"soak",
		"duration_minutes":180,
		"concurrency":180,
		"target_throughput_rps":260,
		"expected_p95_latency_ms":1700
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/tests/load-soak/suites", bytes.NewReader(createSuite))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upsert load-soak suite failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	runBody := []byte(`{"suite_id":"soak-workers-nightly","seed":42,"triggered_by":"ci"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/tests/load-soak/runs", bytes.NewReader(runBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK && rr.Code != http.StatusConflict {
		t.Fatalf("run load-soak suite failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var run struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode load-soak run failed: %v", err)
	}
	if run.ID == "" {
		t.Fatalf("expected run id, body=%s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/release/tests/load-soak/runs?suite_id=soak-workers-nightly&limit=5", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list load-soak runs failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/release/tests/load-soak/runs/"+run.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get load-soak run failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
