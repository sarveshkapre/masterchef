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

func TestTenantCryptoEndpoints(t *testing.T) {
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

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/security/tenant-keys", bytes.NewReader([]byte(`{"tenant":"tenant-a","algorithm":"aes-256-gcm"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ensure tenant key failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var key struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &key)
	if key.ID == "" {
		t.Fatalf("expected key id")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/security/tenant-keys/boundary-check", bytes.NewReader([]byte(`{"request_tenant":"tenant-a","context_tenant":"tenant-a","key_id":"`+key.ID+`","operation":"decrypt"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("tenant boundary check should allow same tenant access: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/security/tenant-keys/boundary-check", bytes.NewReader([]byte(`{"request_tenant":"tenant-a","context_tenant":"tenant-b","key_id":"`+key.ID+`"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("tenant boundary check should reject cross-tenant access: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
