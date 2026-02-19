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

func TestOIDCWorkloadEndpoints(t *testing.T) {
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

	createProvider := []byte(`{
  "name":"aws-oidc",
  "issuer_url":"https://issuer.example.com",
  "audience":"masterchef",
  "jwks_url":"https://issuer.example.com/jwks",
  "allowed_service_accounts":["sa-prod-deployer"]
}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/identity/oidc/workload/providers", bytes.NewReader(createProvider))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create oidc provider failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var provider struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &provider)

	exchange := []byte(`{
  "provider_id":"` + provider.ID + `",
  "subject_token":"id-token",
  "service_account":"sa-prod-deployer",
  "workload":"payments-api"
}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/identity/oidc/workload/exchange", bytes.NewReader(exchange))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("exchange oidc credential failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var cred struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &cred)

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/identity/oidc/workload/credentials/"+cred.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get oidc credential failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
