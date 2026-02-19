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

func TestTicketIntegrationEndpoints(t *testing.T) {
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

	changeBody := []byte(`{"summary":"deploy","description":"ticket sync","requested_by":"sre","approvers":["lead"],"approval_mode":"any","environment":"prod"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/change-records", bytes.NewReader(changeBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create change record failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var change map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &change); err != nil {
		t.Fatalf("decode change record response failed: %v", err)
	}
	changeID, _ := change["id"].(string)
	if changeID == "" {
		t.Fatalf("expected change record id in response: %s", rr.Body.String())
	}

	integrationBody := []byte(`{"name":"jira-prod","provider":"jira","base_url":"https://tickets.example.com","project_key":"OPS","enabled":true}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/change-records/ticket-integrations", bytes.NewReader(integrationBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create ticket integration failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	syncBody := []byte(`{"integration_id":"ticket-integration-1","change_record_id":"` + changeID + `","ticket_id":"42","status":"approved"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/change-records/tickets/sync", bytes.NewReader(syncBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("sync ticket integration failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
