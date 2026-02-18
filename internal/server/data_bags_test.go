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

func TestDataBagEndpoints(t *testing.T) {
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

	plainBody := []byte(`{
		"bag":"apps",
		"item":"payments",
		"data":{"owner":"sre-payments","tier":"critical"},
		"tags":["prod","payments"]
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/data-bags", bytes.NewReader(plainBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create plaintext data bag failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/data-bags?bag=apps", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list data bags failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var listResp struct {
		Count int `json:"count"`
		Items []struct {
			Bag string `json:"bag"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response failed: %v", err)
	}
	if listResp.Count != 1 || len(listResp.Items) != 1 || listResp.Items[0].Bag != "apps" {
		t.Fatalf("unexpected list response: %+v", listResp)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/data-bags/apps/payments", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get plaintext data bag failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var getPlain struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &getPlain); err != nil {
		t.Fatalf("decode get plaintext failed: %v", err)
	}
	if getPlain.Data["owner"] != "sre-payments" {
		t.Fatalf("unexpected plaintext payload: %#v", getPlain.Data)
	}

	encryptedBody := []byte(`{
		"data":{"username":"svc","password":"secret"},
		"encrypted":true,
		"passphrase":"vault-pass",
		"tags":["secret"]
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/v1/data-bags/secrets/db", bytes.NewReader(encryptedBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create encrypted data bag failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/data-bags/secrets/db", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected missing passphrase to fail: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/data-bags/secrets/db?passphrase=vault-pass", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get encrypted data bag failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var getEncrypted struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &getEncrypted); err != nil {
		t.Fatalf("decode get encrypted failed: %v", err)
	}
	if getEncrypted.Data["username"] != "svc" {
		t.Fatalf("unexpected encrypted payload: %#v", getEncrypted.Data)
	}

	searchBody := []byte(`{"bag":"secrets","field":"username","equals":"svc","passphrase":"vault-pass"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/data-bags/search", bytes.NewReader(searchBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("data bag search failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var searchResp struct {
		Count int `json:"count"`
		Items []struct {
			Bag  string `json:"bag"`
			Item string `json:"item"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &searchResp); err != nil {
		t.Fatalf("decode search response failed: %v", err)
	}
	if searchResp.Count != 1 || len(searchResp.Items) != 1 || searchResp.Items[0].Item != "db" {
		t.Fatalf("unexpected search response: %+v", searchResp)
	}

	queryBody := []byte(`{"entity":"data_bag_items","mode":"human","query":"bag=secrets","limit":10}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader(queryBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("query data_bag_items failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var queryResp struct {
		MatchedCount int `json:"matched_count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &queryResp); err != nil {
		t.Fatalf("decode query response failed: %v", err)
	}
	if queryResp.MatchedCount != 1 {
		t.Fatalf("expected one query result, got %d", queryResp.MatchedCount)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/v1/data-bags/apps/payments", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete data bag failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
