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

func TestEncryptedVariableFileEndpoints(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/v1/vars/encrypted/keys", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get key status failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	putBody := []byte(`{
		"name":"prod",
		"data":{"api_key":"abc","region":"us-east-1"},
		"passphrase":"v1-pass"
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/vars/encrypted/files", bytes.NewReader(putBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create encrypted vars failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/vars/encrypted/files/prod?passphrase=v1-pass", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get encrypted vars failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var getResp struct {
		File struct {
			KeyVersion int `json:"key_version"`
		} `json:"file"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode get response failed: %v", err)
	}
	if getResp.File.KeyVersion != 1 || getResp.Data["api_key"] != "abc" {
		t.Fatalf("unexpected get response: %+v", getResp)
	}

	rotateBody := []byte(`{"old_passphrase":"v1-pass","new_passphrase":"v2-pass"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/vars/encrypted/keys", bytes.NewReader(rotateBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("rotate key failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var rotateResp struct {
		CurrentKeyVersion int `json:"current_key_version"`
		RotatedFiles      int `json:"rotated_files"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &rotateResp); err != nil {
		t.Fatalf("decode rotate response failed: %v", err)
	}
	if rotateResp.CurrentKeyVersion != 2 || rotateResp.RotatedFiles != 1 {
		t.Fatalf("unexpected rotate response: %+v", rotateResp)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/vars/encrypted/files/prod?passphrase=v1-pass", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected old passphrase failure after rotation: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/vars/encrypted/files/prod?passphrase=v2-pass", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected get with new passphrase success: code=%d body=%s", rr.Code, rr.Body.String())
	}

	queryBody := []byte(`{"entity":"encrypted_variable_files","mode":"human","query":"name=prod","limit":10}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader(queryBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("query encrypted variable files failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var queryResp struct {
		MatchedCount int `json:"matched_count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &queryResp); err != nil {
		t.Fatalf("decode query response failed: %v", err)
	}
	if queryResp.MatchedCount != 1 {
		t.Fatalf("expected one encrypted variable file match, got %d", queryResp.MatchedCount)
	}
}
