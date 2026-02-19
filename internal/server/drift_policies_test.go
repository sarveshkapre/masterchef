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
	"time"

	"github.com/masterchef/masterchef/internal/state"
)

func TestDriftPolicyEndpointsAndInsightsFiltering(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
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
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })

	suppressionReq := []byte(`{"scope_type":"host","scope_value":"node-a","reason":"maintenance","created_by":"sre","until":"` + time.Now().UTC().Add(10*time.Minute).Format(time.RFC3339) + `"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/drift/suppressions", bytes.NewReader(suppressionReq))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create suppression failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var suppression struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &suppression); err != nil {
		t.Fatalf("decode suppression failed: %v", err)
	}
	if suppression.ID == "" {
		t.Fatalf("expected suppression id")
	}

	allowReq := []byte(`{"scope_type":"resource_type","scope_value":"file","reason":"known benign","created_by":"platform"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/drift/allowlists", bytes.NewReader(allowReq))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create allowlist failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var allow struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &allow); err != nil {
		t.Fatalf("decode allowlist failed: %v", err)
	}
	if allow.ID == "" {
		t.Fatalf("expected allowlist id")
	}

	st := state.New(tmp)
	now := time.Now().UTC()
	if err := st.SaveRun(state.RunRecord{
		ID:        "run-suppressed",
		StartedAt: now.Add(-10 * time.Minute),
		EndedAt:   now.Add(-9 * time.Minute),
		Status:    state.RunSucceeded,
		Results: []state.ResourceRun{
			{ResourceID: "cmd-1", Type: "command", Host: "node-a", Changed: true, Message: "changed"},
		},
	}); err != nil {
		t.Fatalf("save suppressed run failed: %v", err)
	}
	if err := st.SaveRun(state.RunRecord{
		ID:        "run-allowlisted",
		StartedAt: now.Add(-8 * time.Minute),
		EndedAt:   now.Add(-7 * time.Minute),
		Status:    state.RunSucceeded,
		Results: []state.ResourceRun{
			{ResourceID: "file-1", Type: "file", Host: "node-b", Changed: true, Message: "changed"},
		},
	}); err != nil {
		t.Fatalf("save allowlisted run failed: %v", err)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/drift/insights?hours=24", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("drift insights failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var insights map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &insights); err != nil {
		t.Fatalf("decode drift insights failed: %v", err)
	}
	if insights["suppressed_changes"].(float64) != 1 {
		t.Fatalf("expected one suppressed change, got %#v", insights["suppressed_changes"])
	}
	if insights["allowlisted_changes"].(float64) != 1 {
		t.Fatalf("expected one allowlisted change, got %#v", insights["allowlisted_changes"])
	}
	if insights["total_changed_resources"].(float64) != 0 {
		t.Fatalf("expected no remaining drift changes, got %#v", insights["total_changed_resources"])
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/v1/drift/suppressions/"+suppression.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete suppression failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
