package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/masterchef/masterchef/internal/state"
)

func TestDriftSLOEndpoints(t *testing.T) {
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

	now := time.Now().UTC()
	run := state.RunRecord{
		ID:        "drift-slo-run-1",
		StartedAt: now.Add(-30 * time.Minute),
		EndedAt:   now.Add(-20 * time.Minute),
		Status:    state.RunSucceeded,
		Results:   make([]state.ResourceRun, 0, 30),
	}
	for i := 0; i < 30; i++ {
		run.Results = append(run.Results, state.ResourceRun{
			ResourceID: "res-" + strconv.Itoa(i),
			Type:       "file",
			Host:       "node-a",
			Changed:    i < 10,
			Skipped:    false,
			Message:    "ok",
		})
	}
	if err := state.New(tmp).SaveRun(run); err != nil {
		t.Fatalf("save run failed: %v", err)
	}

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	policyBody := []byte(`{"target_percent":90,"window_hours":24,"min_samples":10,"auto_create_incident":true,"incident_hook":"event://incident.create.drift"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/drift/slo/policy", bytes.NewReader(policyBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set drift slo policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/drift/slo/evaluate", bytes.NewReader([]byte(`{}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected drift slo breach conflict: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"breached"`) || !strings.Contains(rr.Body.String(), `"incident_recommended":true`) {
		t.Fatalf("expected breached evaluation response: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/drift/slo/evaluations?limit=5", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list drift slo evaluations failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "drift-slo-eval-") {
		t.Fatalf("expected evaluation id in list response: %s", rr.Body.String())
	}
}
