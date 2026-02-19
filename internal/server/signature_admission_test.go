package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSignatureAdmissionEndpoints(t *testing.T) {
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

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	createKey := []byte(`{"name":"release-key","algorithm":"ed25519","public_key":"` + base64.StdEncoding.EncodeToString(pub) + `","scopes":["image","collection"]}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/security/signatures/keyrings", bytes.NewReader(createKey))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create signature key failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var key struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &key); err != nil {
		t.Fatalf("decode key failed: %v", err)
	}

	setPolicy := []byte(`{"require_signed_scopes":["image"],"trusted_key_ids":["` + key.ID + `"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/security/signatures/admission-policy", bytes.NewReader(setPolicy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set signature policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	digest := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	payload := controlCanonicalPayloadForServerTest("image", "ghcr.io/masterchef/runtime", digest)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(payload)))
	admit := []byte(`{"scope":"image","artifact_ref":"ghcr.io/masterchef/runtime","digest":"` + digest + `","key_id":"` + key.ID + `","signature":"` + signature + `"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/security/signatures/admit-check", bytes.NewReader(admit))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected signed image to pass admission: code=%d body=%s", rr.Code, rr.Body.String())
	}

	badSig := []byte(`{"scope":"image","artifact_ref":"ghcr.io/masterchef/runtime","digest":"` + digest + `","key_id":"` + key.ID + `","signature":"` + base64.StdEncoding.EncodeToString([]byte("bad")) + `"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/security/signatures/admit-check", bytes.NewReader(badSig))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected invalid signature to fail admission: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func controlCanonicalPayloadForServerTest(scope, artifactRef, digest string) string {
	return scope + "|" + artifactRef + "|" + digest
}
