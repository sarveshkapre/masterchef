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

func TestPropertyHarnessEndpoints(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/v1/release/tests/property-harness/cases", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list property harness cases failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	createCase := []byte(`{
		"id":"property-service-case",
		"name":"Service Property Case",
		"provider":"service",
		"resource_type":"service",
		"invariants":["idempotent_restart","converges"],
		"generated_samples":90
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/tests/property-harness/cases", bytes.NewReader(createCase))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upsert property harness case failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	runBody := []byte(`{"case_id":"property-service-case","seed":42,"triggered_by":"ci"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/tests/property-harness/runs", bytes.NewReader(runBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK && rr.Code != http.StatusConflict {
		t.Fatalf("run property harness failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var run struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode property run failed: %v", err)
	}
	if run.ID == "" {
		t.Fatalf("expected run id, body=%s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/release/tests/property-harness/runs?case_id=property-service-case&limit=5", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list property harness runs failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/release/tests/property-harness/runs/"+run.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get property harness run failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
