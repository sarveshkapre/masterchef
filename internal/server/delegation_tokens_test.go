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

func TestDelegationTokenEndpoints(t *testing.T) {
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

	issue := []byte(`{"grantor":"platform-admin","delegatee":"pipeline:release-42","pipeline_id":"release-42","scopes":["run:apply"],"ttl_seconds":120,"max_uses":1}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/access/delegation-tokens", bytes.NewReader(issue))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("issue delegation token failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var issued struct {
		Token      string `json:"token"`
		Delegation struct {
			ID string `json:"id"`
		} `json:"delegation"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &issued); err != nil {
		t.Fatalf("decode issue response failed: %v", err)
	}
	if issued.Token == "" || issued.Delegation.ID == "" {
		t.Fatalf("expected token and delegation id")
	}

	validate := []byte(`{"token":"` + issued.Token + `","required_scope":"run:apply"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/delegation-tokens/validate", bytes.NewReader(validate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("validate delegation token failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/delegation-tokens/validate", bytes.NewReader(validate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected exhausted token to fail validation: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/delegation-tokens/"+issued.Delegation.ID+"/revoke", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke delegation token failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
