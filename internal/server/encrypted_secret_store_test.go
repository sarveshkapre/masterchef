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

func TestEncryptedSecretStoreEndpoints(t *testing.T) {
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

	createBody := []byte(`{"name":"db_password","value":"secret-v1","ttl_seconds":3600}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/secrets/encrypted-store/items", bytes.NewReader(createBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create encrypted secret failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/secrets/encrypted-store/items/db_password", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get encrypted secret failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/secrets/encrypted-store/items/db_password/resolve", bytes.NewReader([]byte(`{}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("resolve encrypted secret failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var resolved map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resolved); err != nil {
		t.Fatalf("decode resolve response failed: %v", err)
	}
	if resolved["value"] != "secret-v1" {
		t.Fatalf("unexpected resolved value: %s", rr.Body.String())
	}

	rotateBody := []byte(`{"value":"secret-v2","extend_ttl_seconds":7200}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/secrets/encrypted-store/items/db_password/rotate", bytes.NewReader(rotateBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("rotate encrypted secret failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/secrets/encrypted-store/expired", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list expired encrypted secrets failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
