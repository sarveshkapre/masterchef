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

func TestCanonicalizationEndpoint(t *testing.T) {
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

	firstBody := []byte(`{"kind":"config","content":{"resources":[{"id":"a","type":"command","host":"local","command":"echo hi"}],"vars":{"z":"2","a":"1"}}}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/format/canonicalize", bytes.NewReader(firstBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("canonicalize first config failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var first struct {
		SHA string `json:"canonical_sha256"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first canonical response failed: %v", err)
	}

	secondBody := []byte(`{"kind":"config","content":{"vars":{"a":"1","z":"2"},"resources":[{"command":"echo hi","host":"local","type":"command","id":"a"}]}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/format/canonicalize", bytes.NewReader(secondBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("canonicalize second config failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var second struct {
		SHA string `json:"canonical_sha256"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second canonical response failed: %v", err)
	}
	if first.SHA == "" || first.SHA != second.SHA {
		t.Fatalf("expected canonical hashes to match: first=%s second=%s", first.SHA, second.SHA)
	}

	planBody := []byte(`{"kind":"plan","content":{"steps":[{"action":"apply","resource":{"id":"x","type":"command","host":"local","command":"echo hi"}}]}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/format/canonicalize", bytes.NewReader(planBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("canonicalize plan failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
