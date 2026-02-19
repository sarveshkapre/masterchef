package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutionEnvironmentEndpoints(t *testing.T) {
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

	create := []byte(`{
  "name":"hermetic-go",
  "image_digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111",
  "dependencies":["go1.23.0","apk@3.20"],
  "signed":true,
  "signature_ref":"sigstore://abc"
}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/execution/environments", bytes.NewReader(create))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create execution env failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create env failed: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected env id in response")
	}

	policy := []byte(`{"require_signed":true,"allowed_digests":["sha256:1111111111111111111111111111111111111111111111111111111111111111"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/admission-policy", bytes.NewReader(policy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set admission policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	admit := []byte(`{"environment_id":"` + created.ID + `"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/admit-check", bytes.NewReader(admit))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"allowed":true`) {
		t.Fatalf("admission check expected allow: code=%d body=%s", rr.Code, rr.Body.String())
	}

	createUnsigned := []byte(`{
  "name":"unsigned",
  "image_digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222",
  "signed":false
}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/environments", bytes.NewReader(createUnsigned))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create unsigned env failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var unsigned struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &unsigned)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/admit-check", bytes.NewReader([]byte(`{"environment_id":"`+unsigned.ID+`"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected unsigned env to fail admission: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
