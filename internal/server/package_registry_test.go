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

func TestPackageRegistryEndpoints(t *testing.T) {
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

	publish := []byte(`{
  "kind":"module",
  "name":"core/network",
  "version":"1.2.3",
  "digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111",
  "signed":true,
  "key_id":"sigkey-1",
  "signature":"sig",
  "provenance":{"source_repo":"github.com/masterchef/modules","source_ref":"refs/tags/v1.2.3","builder":"gha://build/1"}
}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/packages/artifacts", bytes.NewReader(publish))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("publish package artifact failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var artifact struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &artifact)

	policy := []byte(`{"require_signed":true,"trusted_key_ids":["sigkey-1"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/packages/signing-policy", bytes.NewReader(policy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set signing policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	verify := []byte(`{"artifact_id":"` + artifact.ID + `"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/packages/verify", bytes.NewReader(verify))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("verify package artifact failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	certPolicy := []byte(`{"require_conformance":true,"min_test_pass_rate":0.95,"max_high_vulns":0,"max_critical_vulns":0,"require_signed":true,"min_maintainer_score":80}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/packages/certification-policy", bytes.NewReader(certPolicy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set certification policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	certify := []byte(`{"artifact_id":"` + artifact.ID + `","conformance_passed":true,"test_pass_rate":0.99,"high_vulnerabilities":0,"critical_vulnerabilities":0,"maintainer_score":92}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/packages/certify", bytes.NewReader(certify))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("certify package artifact failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	pubCheck := []byte(`{"artifact_id":"` + artifact.ID + `","target":"public"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/packages/publication/check", bytes.NewReader(pubCheck))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected public gate failure without sbom/attestation digests: code=%d body=%s", rr.Code, rr.Body.String())
	}

	health := []byte(`{"maintainer":"platform-team","test_pass_rate":0.99,"issue_latency_hours":4,"release_cadence_days":7,"open_security_issues":0}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/packages/maintainers/health", bytes.NewReader(health))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("maintainer health upsert failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/packages/maintainers/health/platform-team", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("maintainer health get failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
