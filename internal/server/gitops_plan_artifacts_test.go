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

	"github.com/masterchef/masterchef/internal/policy"
)

func TestGitOpsPlanArtifactSignVerifyEndpoints(t *testing.T) {
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
	pub, priv, err := policy.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.SavePrivateKey(filepath.Join(tmp, "policy-private.key"), priv); err != nil {
		t.Fatal(err)
	}
	if err := policy.SavePublicKey(filepath.Join(tmp, "policy-public.key"), pub); err != nil {
		t.Fatal(err)
	}

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	signBody := []byte(`{"config_path":"c.yaml","private_key_path":"policy-private.key"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/gitops/plan-artifacts/sign", bytes.NewReader(signBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("sign endpoint failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var bundle policy.Bundle
	if err := json.Unmarshal(rr.Body.Bytes(), &bundle); err != nil {
		t.Fatalf("decode bundle failed: %v", err)
	}
	if bundle.Signature == "" {
		t.Fatalf("expected signature in bundle: %+v", bundle)
	}

	verifyBody := []byte(`{"public_key_path":"policy-public.key","bundle":` + rr.Body.String() + `}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/gitops/plan-artifacts/verify", bytes.NewReader(verifyBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("verify endpoint failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"verified":true`) {
		t.Fatalf("expected verified true response: %s", rr.Body.String())
	}
}
