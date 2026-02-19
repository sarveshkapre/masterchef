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

func TestAgentPKIEndpoints(t *testing.T) {
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

	policy := []byte(`{"auto_approve":true,"required_attributes":{"env":"prod"}}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/cert-policy", bytes.NewReader(policy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set cert policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	submitCSR := []byte(`{"agent_id":"agent-1","attributes":{"env":"prod"}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/csrs", bytes.NewReader(submitCSR))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("submit csr failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var csr struct {
		Status string `json:"status"`
		CertID string `json:"cert_id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &csr)
	if csr.Status != "issued" || csr.CertID == "" {
		t.Fatalf("expected auto-issued csr, got %+v", csr)
	}

	rotate := []byte(`{"agent_id":"agent-1"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/certificates/rotate", bytes.NewReader(rotate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("rotate cert failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var cert struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &cert)

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/certificates/"+cert.ID+"/revoke", bytes.NewReader([]byte(`{}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke cert failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
