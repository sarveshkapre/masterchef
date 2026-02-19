package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCosignVerificationEndpoints(t *testing.T) {
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

	rootBody := []byte(`{
		"name":"prod-root",
		"issuer":"https://token.actions.githubusercontent.com",
		"subject":"repo:masterchef/masterchef",
		"rekor_public_key_ref":"rekor://prod",
		"transparency_log_url":"https://rekor.sigstore.dev",
		"enabled":true
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/packages/cosign/trust-roots", bytes.NewReader(rootBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upsert cosign trust root failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	policyBody := []byte(`{
		"require_transparency_log":true,
		"allowed_issuers":["https://token.actions.githubusercontent.com"],
		"allowed_subjects":["repo:masterchef/masterchef"]
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/packages/cosign/policy", bytes.NewReader(policyBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set cosign policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	verifyBody := []byte(`{
		"artifact_ref":"ghcr.io/masterchef/control-plane:v1.0.0",
		"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"signature":"cosign:bundle-ref",
		"trusted_root_id":"cosign-root-1",
		"issuer":"https://token.actions.githubusercontent.com",
		"subject":"repo:masterchef/masterchef",
		"transparency_log_index":42
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/packages/cosign/verify", bytes.NewReader(verifyBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("cosign verify failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
