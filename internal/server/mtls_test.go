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

func TestMTLSEndpoints(t *testing.T) {
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

	createCA := []byte(`{"name":"prod-ca","ca_bundle":"-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/security/mtls/authorities", bytes.NewReader(createCA))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create mtls authority failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var authority struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &authority)

	setPolicy := []byte(`{"component":"control-plane","min_tls_version":"1.3","require_client_cert":true,"allowed_authorities":["` + authority.ID + `"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/security/mtls/policies", bytes.NewReader(setPolicy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set mtls policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	checkOK := []byte(`{"component":"control-plane","authority_id":"` + authority.ID + `","tls_version":"1.3","client_cert":true}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/security/mtls/handshake-check", bytes.NewReader(checkOK))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected mtls check success: code=%d body=%s", rr.Code, rr.Body.String())
	}

	checkFail := []byte(`{"component":"control-plane","authority_id":"` + authority.ID + `","tls_version":"1.2","client_cert":true}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/security/mtls/handshake-check", bytes.NewReader(checkFail))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected mtls check failure: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
