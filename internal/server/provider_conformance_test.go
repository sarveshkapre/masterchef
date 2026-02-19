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

func TestProviderConformanceEndpoints(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/v1/providers/conformance/suites", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list suites failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	createSuite := []byte(`{
		"id":"provider-network-core",
		"provider":"network",
		"description":"network provider conformance checks",
		"checks":["connect","idempotency","rollback"],
		"required_pass_rate":0.9
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/providers/conformance/suites", bytes.NewReader(createSuite))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upsert suite failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	runBody := []byte(`{"suite_id":"provider-network-core","provider_version":"v2.1.0","trigger":"nightly"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/providers/conformance/runs", bytes.NewReader(runBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("run conformance failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var run struct {
		ID      string `json:"id"`
		SuiteID string `json:"suite_id"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode run response failed: %v", err)
	}
	if run.ID == "" || run.SuiteID != "provider-network-core" || run.Status == "" {
		t.Fatalf("unexpected run response: %+v", run)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/providers/conformance/runs?suite_id=provider-network-core&limit=5", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list runs failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/providers/conformance/runs/"+run.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get run failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
