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

func TestAccessApprovalAndBreakGlassEndpoints(t *testing.T) {
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
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	createPolicy := []byte(`{"name":"prod-sensitive","stages":[{"name":"peer","required_approvals":1},{"name":"security","required_approvals":2}]}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/access/approval-policies", bytes.NewReader(createPolicy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create approval policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var policy struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &policy); err != nil {
		t.Fatalf("decode policy failed: %v", err)
	}
	if policy.ID == "" {
		t.Fatalf("expected policy id")
	}

	createRequest := []byte(`{"requested_by":"oncall","reason":"prod outage","scope":"service/payments","policy_id":"` + policy.ID + `","ttl_seconds":600}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/break-glass/requests", bytes.NewReader(createRequest))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create break-glass request failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var bg struct {
		ID           string `json:"id"`
		Status       string `json:"status"`
		CurrentStage int    `json:"current_stage"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &bg); err != nil {
		t.Fatalf("decode break-glass request failed: %v", err)
	}
	if bg.ID == "" || bg.Status != "pending" {
		t.Fatalf("unexpected break-glass create response: %+v", bg)
	}

	approve1 := []byte(`{"actor":"peer-1","comment":"approved"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/break-glass/requests/"+bg.ID+"/approve", bytes.NewReader(approve1))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("stage-1 approve failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	approve2 := []byte(`{"actor":"security-1","comment":"approved"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/break-glass/requests/"+bg.ID+"/approve", bytes.NewReader(approve2))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("stage-2 first approve failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	approve3 := []byte(`{"actor":"security-2","comment":"approved"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/break-glass/requests/"+bg.ID+"/approve", bytes.NewReader(approve3))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("stage-2 second approve failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var active struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &active); err != nil {
		t.Fatalf("decode active response failed: %v", err)
	}
	if active.Status != "active" {
		t.Fatalf("expected active break-glass status, got %+v", active)
	}

	revoke := []byte(`{"actor":"incident-commander","comment":"incident resolved"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/break-glass/requests/"+bg.ID+"/revoke", bytes.NewReader(revoke))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke break-glass request failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
