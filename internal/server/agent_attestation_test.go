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

func TestAgentAttestationEndpointsAndCSRGuard(t *testing.T) {
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

	certPolicy := []byte(`{"auto_approve":true,"required_attributes":{"env":"prod"}}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/cert-policy", bytes.NewReader(certPolicy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set cert policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	attestationPolicy := []byte(`{"require_before_cert":true,"allowed_providers":["tpm"],"max_age_minutes":60}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/attestation/policy", bytes.NewReader(attestationPolicy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set attestation policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	submitCSR := []byte(`{"agent_id":"agent-1","attributes":{"env":"prod"}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/csrs", bytes.NewReader(submitCSR))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected csr blocked without attestation: code=%d body=%s", rr.Code, rr.Body.String())
	}

	submitAttestation := []byte(`{"agent_id":"agent-1","provider":"tpm","nonce":"nonce-1"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/attestations", bytes.NewReader(submitAttestation))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("submit attestation failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var attestation struct {
		ID       string `json:"id"`
		Verified bool   `json:"verified"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &attestation)
	if attestation.ID == "" || !attestation.Verified {
		t.Fatalf("expected verified attestation, got %+v", attestation)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/agents/attestations/"+attestation.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get attestation failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/attestations/check", bytes.NewReader([]byte(`{"agent_id":"agent-1"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("attestation check failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var check struct {
		Allowed bool `json:"allowed"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &check)
	if !check.Allowed {
		t.Fatalf("expected allowed attestation check, got %+v", check)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/csrs", bytes.NewReader(submitCSR))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("submit csr after attestation failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var csr1 struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &csr1)
	if csr1.Status != "issued" {
		t.Fatalf("expected auto-issued csr, got %+v", csr1)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/cert-policy", bytes.NewReader([]byte(`{"auto_approve":false}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("disable auto-approve failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/attestation/policy", bytes.NewReader([]byte(`{"require_before_cert":false}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("disable attestation requirement failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/csrs", bytes.NewReader([]byte(`{"agent_id":"agent-2"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("submit pending csr failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var csr2 struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &csr2)
	if csr2.ID == "" {
		t.Fatalf("expected csr id for agent-2")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/attestation/policy", bytes.NewReader(attestationPolicy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("re-enable attestation requirement failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/csrs/"+csr2.ID+"/approve", bytes.NewReader([]byte(`{"reason":"manual"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected approve blocked without attestation: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/attestations", bytes.NewReader([]byte(`{"agent_id":"agent-2","provider":"tpm","nonce":"nonce-2"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("submit attestation for agent-2 failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/csrs/"+csr2.ID+"/approve", bytes.NewReader([]byte(`{"reason":"manual"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("approve after attestation failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
