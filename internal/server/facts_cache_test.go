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

func TestFactCacheAndSaltMineEndpoints(t *testing.T) {
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

	putBody := []byte(`{
		"node":"web-01",
		"facts":{"role":"web","meta":{"zone":"us-east-1a","owner":"platform"}},
		"ttl_seconds":60
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/facts/cache", bytes.NewReader(putBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("facts upsert failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/facts/cache?field=meta.zone&contains=us-east", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("facts list/query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var listResp struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode facts list failed: %v", err)
	}
	if listResp.Count != 1 {
		t.Fatalf("expected one fact record, got %d", listResp.Count)
	}

	mineBody := []byte(`{"field":"role","equals":"web","limit":10}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/facts/mine/query", bytes.NewReader(mineBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("mine query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var mineResp struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &mineResp); err != nil {
		t.Fatalf("decode mine response failed: %v", err)
	}
	if mineResp.Count != 1 {
		t.Fatalf("expected one mine match, got %d", mineResp.Count)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/facts/cache/web-01", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("facts get by node failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	queryBody := []byte(`{"entity":"facts_cache","mode":"human","query":"node=web-01","limit":10}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader(queryBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("query facts_cache failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var queryResp struct {
		MatchedCount int `json:"matched_count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &queryResp); err != nil {
		t.Fatalf("decode query response failed: %v", err)
	}
	if queryResp.MatchedCount != 1 {
		t.Fatalf("expected one query match, got %d", queryResp.MatchedCount)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/v1/facts/cache/web-01", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("facts delete failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
