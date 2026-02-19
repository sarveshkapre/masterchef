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

func TestReadinessScorecardEndpoints(t *testing.T) {
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

	passBody := []byte(`{
		"environment":"prod",
		"service":"payments-api",
		"owner":"sre-payments",
		"signals":{
			"quality_score":0.97,
			"reliability_score":0.96,
			"performance_score":0.95,
			"test_pass_rate":0.995,
			"flake_rate":0.005,
			"open_critical_incidents":0,
			"p95_apply_latency_ms":1000
		}
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/release/readiness/scorecards", bytes.NewReader(passBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create readiness scorecard failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created readiness scorecard failed: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected scorecard id, body=%s", rr.Body.String())
	}

	failBody := []byte(`{
		"environment":"prod",
		"service":"search-api",
		"signals":{
			"quality_score":0.5,
			"reliability_score":0.5,
			"performance_score":0.5,
			"test_pass_rate":0.7,
			"flake_rate":0.3,
			"open_critical_incidents":2,
			"p95_apply_latency_ms":999999
		}
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/readiness/scorecards", bytes.NewReader(failBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected failing readiness scorecard conflict (409): code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/release/readiness/scorecards?environment=prod&limit=10", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list readiness scorecards failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/release/readiness/scorecards/"+created.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get readiness scorecard failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
