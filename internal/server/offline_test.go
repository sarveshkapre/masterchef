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

	"github.com/masterchef/masterchef/internal/policy"
)

func TestOfflineModeAndBundles(t *testing.T) {
	tmp := t.TempDir()
	features := filepath.Join(tmp, "features.md")
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
	if err := policy.SavePrivateKey(filepath.Join(tmp, "offline-private.key"), priv); err != nil {
		t.Fatal(err)
	}
	if err := policy.SavePublicKey(filepath.Join(tmp, "offline-public.key"), pub); err != nil {
		t.Fatal(err)
	}

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/offline/mode", bytes.NewReader([]byte(`{"enabled":true,"air_gapped":true,"mirror_path":"/mnt/mirror"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set offline mode failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	bundleReq := []byte(`{"items":["policy/main.yaml","modules/pkg.tgz"],"artifacts":["registry/pkg@sha256:abc"],"private_key_path":"offline-private.key"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/offline/bundles", bytes.NewReader(bundleReq))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create offline bundle failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var bundle struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &bundle)
	if bundle.ID == "" {
		t.Fatalf("expected bundle id")
	}

	verifyReq := []byte(`{"bundle_id":"` + bundle.ID + `","public_key_path":"offline-public.key"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/offline/bundles/verify", bytes.NewReader(verifyReq))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("verify offline bundle failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
