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

func TestCompatibilityShimEndpoints(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/v1/compat/shims", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list shims failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var listed []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list shims failed: %v", err)
	}
	if len(listed) == 0 {
		t.Fatalf("expected built-in shims")
	}

	upsertBody := []byte(`{
		"source_platform":"ansible",
		"legacy_pattern":"legacy include_tasks wrapper",
		"description":"Map include_tasks wrappers to explicit plan templates",
		"target":"workflow template",
		"keywords":["include_tasks","import_tasks"],
		"risk_level":"medium",
		"recommendation":"convert include_tasks wrappers to typed workflow templates",
		"convergence_safe":true
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/compat/shims", bytes.NewReader(upsertBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upsert shim failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created shim failed: %v", err)
	}
	if created.ID == "" || !created.Enabled {
		t.Fatalf("unexpected created shim response: %+v", created)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/compat/shims/"+created.ID+"/disable", bytes.NewReader([]byte(`{}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("disable shim failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	resolveBody := []byte(`{
		"source_platform":"ansible",
		"content":"- include_tasks: app.yml\n  when: result.changed",
		"limit":5
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/compat/shims/resolve", bytes.NewReader(resolveBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("resolve shim failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var resolve struct {
		CoverageScore int `json:"coverage_score"`
		Matched       []struct {
			Shim struct {
				ID string `json:"id"`
			} `json:"shim"`
		} `json:"matched"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resolve); err != nil {
		t.Fatalf("decode resolve response failed: %v", err)
	}
	if len(resolve.Matched) == 0 || resolve.CoverageScore <= 0 {
		t.Fatalf("expected resolve matches with coverage score, got %+v", resolve)
	}
}
