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

func TestRuntimeSecretEndpoints(t *testing.T) {
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

	upsert := []byte(`{"name":"prod","data":{"db_user":"svc","db_pass":"secret"},"passphrase":"vault-v1"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/vars/encrypted/files", bytes.NewReader(upsert))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create encrypted vars failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	createSession := []byte(`{"source":"prod","passphrase":"vault-v1","ttl_seconds":120}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/secrets/runtime/sessions", bytes.NewReader(createSession))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("materialize runtime secret failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var session struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode runtime session failed: %v", err)
	}
	if session.ID == "" {
		t.Fatalf("expected runtime session id")
	}

	consume := []byte(`{"session_id":"` + session.ID + `"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/secrets/runtime/consume", bytes.NewReader(consume))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("consume runtime secret failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var consumed struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &consumed); err != nil {
		t.Fatalf("decode consumed response failed: %v", err)
	}
	if consumed.Data["db_pass"] != "secret" {
		t.Fatalf("unexpected consumed data: %#v", consumed.Data)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/secrets/runtime/consume", bytes.NewReader(consume))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected second consume to fail: code=%d body=%s", rr.Code, rr.Body.String())
	}

	createSession = []byte(`{"source":"prod","passphrase":"vault-v1","ttl_seconds":120}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/secrets/runtime/sessions", bytes.NewReader(createSession))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("materialize second runtime secret failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var second struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &second)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/secrets/runtime/sessions/"+second.ID+"/destroy", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("destroy runtime secret failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
