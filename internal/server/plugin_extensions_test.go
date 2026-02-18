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

func TestPluginExtensionEndpoints(t *testing.T) {
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

	createBody := []byte(`{
		"name":"slack-callback",
		"type":"callback",
		"entrypoint":"/plugins/slack/callback.so",
		"enabled":true
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/extensions", bytes.NewReader(createBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create plugin extension failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create plugin response failed: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected created plugin id")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/plugins/extensions?type=callback", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list plugin extensions failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/plugins/extensions/"+created.ID+"/disable", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("disable plugin extension failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	queryBody := []byte(`{"entity":"plugin_extensions","mode":"human","query":"type=callback","limit":10}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader(queryBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("query plugin_extensions failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var queryResp struct {
		MatchedCount int `json:"matched_count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &queryResp); err != nil {
		t.Fatalf("decode query response failed: %v", err)
	}
	if queryResp.MatchedCount != 1 {
		t.Fatalf("expected one plugin extension match, got %d", queryResp.MatchedCount)
	}
}
