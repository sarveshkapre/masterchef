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
}
