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

func TestExecutionCredentialEndpoints(t *testing.T) {
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

	issue := []byte(`{"subject":"runner@prod","scopes":["run:execute","artifact:read"],"ttl_seconds":120}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/execution/credentials", bytes.NewReader(issue))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("issue credential failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var issued struct {
		Credential struct {
			ID string `json:"id"`
		} `json:"credential"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &issued); err != nil {
		t.Fatalf("decode issue response failed: %v", err)
	}
	if issued.Credential.ID == "" || issued.Token == "" {
		t.Fatalf("expected issued credential id and token")
	}

	validate := []byte(`{"token":"` + issued.Token + `","required_scopes":["run:execute"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/credentials/validate", bytes.NewReader(validate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("validate credential failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/credentials/"+issued.Credential.ID+"/revoke", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke credential failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/credentials/validate", bytes.NewReader(validate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected revoked credential validation failure: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
