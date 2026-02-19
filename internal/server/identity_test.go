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

func TestSSOAndSCIMEndpoints(t *testing.T) {
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
  "name":"okta-main",
  "protocol":"oidc",
  "issuer_url":"https://id.example.com",
  "client_id":"masterchef-client",
  "redirect_url":"https://masterchef.example.com/callback",
  "allowed_domains":["example.com"]
}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/identity/sso/providers", bytes.NewReader(createProvider))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create provider failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var provider struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &provider)
	if provider.ID == "" {
		t.Fatalf("expected provider id")
	}

	startLogin := []byte(`{"provider_id":"` + provider.ID + `","email":"alice@example.com"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/identity/sso/login/start", bytes.NewReader(startLogin))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("start login failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var start struct {
		State string `json:"state"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &start)

	completeLogin := []byte(`{"state":"` + start.State + `","code":"ok","subject":"alice","email":"alice@example.com","groups":["sre"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/identity/sso/login/callback", bytes.NewReader(completeLogin))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("complete login failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	upsertRole := []byte(`{"external_id":"role-ext-1","name":"Platform Admin","description":"full access"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/identity/scim/roles", bytes.NewReader(upsertRole))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upsert scim role failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var role struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &role)

	upsertTeam := []byte(`{"external_id":"team-ext-1","name":"Platform","members":["alice@example.com"],"roles":["` + role.ID + `"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/identity/scim/teams", bytes.NewReader(upsertTeam))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upsert scim team failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
