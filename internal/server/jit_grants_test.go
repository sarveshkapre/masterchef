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

func TestJITAccessGrantEndpoints(t *testing.T) {
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

	createPolicy := []byte(`{"name":"single-stage","stages":[{"name":"approval","required_approvals":1}]}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/access/approval-policies", bytes.NewReader(createPolicy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create approval policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var policy struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &policy)

	createBG := []byte(`{"requested_by":"oncall","reason":"prod outage","scope":"service/payments","policy_id":"` + policy.ID + `","ttl_seconds":600}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/break-glass/requests", bytes.NewReader(createBG))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create break-glass request failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var bg struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &bg)

	approve := []byte(`{"actor":"approver","comment":"ok"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/break-glass/requests/"+bg.ID+"/approve", bytes.NewReader(approve))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("approve break-glass request failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	issue := []byte(`{"subject":"oncall","resource":"service/payments","action":"restart","issued_by":"incident-commander","reason":"restore service","break_glass_request_id":"` + bg.ID + `","ttl_seconds":120}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/jit-grants", bytes.NewReader(issue))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("issue jit grant failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var issued struct {
		Token string `json:"token"`
		Grant struct {
			ID string `json:"id"`
		} `json:"grant"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &issued); err != nil {
		t.Fatalf("decode issued jit grant failed: %v", err)
	}
	if issued.Token == "" || issued.Grant.ID == "" {
		t.Fatalf("expected issued jit token and grant id")
	}

	validate := []byte(`{"token":"` + issued.Token + `","resource":"service/payments","action":"restart"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/jit-grants/validate", bytes.NewReader(validate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("validate jit grant failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/jit-grants/"+issued.Grant.ID+"/revoke", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke jit grant failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/access/jit-grants/validate", bytes.NewReader(validate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected revoked jit grant validation failure: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
