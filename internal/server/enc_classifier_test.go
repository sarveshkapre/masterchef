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
)

func TestENCProviderEndpoints(t *testing.T) {
	enc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := map[string]any{
			"classes":     []string{"base", "web"},
			"run_list":    []string{"role[web]"},
			"environment": "prod",
			"attributes":  map[string]any{"owner": "platform"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer enc.Close()

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

	create := []byte(`{"name":"enc-main","endpoint":"` + enc.URL + `","enabled":true}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/inventory/node-classifiers", bytes.NewReader(create))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create enc provider failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var provider struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &provider); err != nil {
		t.Fatalf("decode provider: %v", err)
	}
	if provider.ID == "" {
		t.Fatalf("expected provider id in response")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/inventory/node-classifiers/"+provider.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get enc provider failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	classify := []byte(`{"provider_id":"` + provider.ID + `","node":"web-1","facts":{"os":"linux"},"labels":{"env":"prod"}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/inventory/node-classifiers/classify", bytes.NewReader(classify))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("enc classify failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"classes":["base","web"]`) {
		t.Fatalf("expected classes in classify output: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/inventory/node-classifiers/"+provider.ID+"/disable", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("disable enc provider failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/inventory/node-classifiers/classify", bytes.NewReader(classify))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected classify to fail for disabled provider: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
