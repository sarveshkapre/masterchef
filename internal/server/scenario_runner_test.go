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

func TestScenarioRunnerEndpoints(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/v1/release/tests/scenarios", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list scenarios failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	createScenario := []byte(`{
		"id":"fleet-sim-nightly",
		"name":"Fleet Sim Nightly",
		"description":"nightly fleet stress simulation",
		"fleet_size":1200,
		"services":25,
		"failure_rate":0.03,
		"chaos_level":30
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/tests/scenarios", bytes.NewReader(createScenario))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upsert scenario failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	runBody := []byte(`{"scenario_id":"fleet-sim-nightly","seed":1234,"triggered_by":"ci"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/tests/scenario-runs", bytes.NewReader(runBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("run scenario failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var run struct {
		ID         string `json:"id"`
		ScenarioID string `json:"scenario_id"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode run failed: %v", err)
	}
	if run.ID == "" || run.ScenarioID != "fleet-sim-nightly" || run.Status == "" {
		t.Fatalf("unexpected run payload: %+v", run)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/release/tests/scenario-runs/"+run.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get run failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	baselineBody := []byte(`{"name":"golden-fleet-nightly","run_id":"` + run.ID + `"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/tests/scenario-baselines", bytes.NewReader(baselineBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create baseline failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var baseline struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &baseline); err != nil {
		t.Fatalf("decode baseline failed: %v", err)
	}
	if baseline.ID == "" {
		t.Fatalf("expected baseline id, got body=%s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/release/tests/scenario-baselines/"+baseline.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get baseline failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	runBody = []byte(`{"scenario_id":"fleet-sim-nightly","seed":1235,"baseline_id":"` + baseline.ID + `","triggered_by":"ci"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/tests/scenario-runs", bytes.NewReader(runBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("run with baseline failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var secondRun struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &secondRun); err != nil {
		t.Fatalf("decode second run failed: %v", err)
	}
	if secondRun.ID == "" {
		t.Fatalf("expected second run id, body=%s", rr.Body.String())
	}

	compareBody := []byte(`{"baseline_id":"` + baseline.ID + `"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/tests/scenario-runs/"+secondRun.ID+"/compare-baseline", bytes.NewReader(compareBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("compare baseline failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
