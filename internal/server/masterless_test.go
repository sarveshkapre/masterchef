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

func TestMasterlessModeAndRenderEndpoints(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPost, "/v1/execution/masterless/mode", bytes.NewReader([]byte(`{"enabled":true,"state_root":"/var/lib/masterchef/masterless","default_strategy":"merge-last"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set masterless mode failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	render := []byte(`{
		"state_template":"package={{pillar.packages.nginx}}\nenvironment={{var.environment}}",
		"layers":[{"name":"defaults","data":{"packages":{"nginx":"nginx"}}}],
		"vars":{"environment":"prod"},
		"lookups":["packages.nginx"]
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/masterless/render", bytes.NewReader(render))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("masterless render failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		RenderedState string `json:"rendered_state"`
		Deterministic bool   `json:"deterministic"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if !body.Deterministic || body.RenderedState == "" {
		t.Fatalf("unexpected render response %+v", body)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/masterless/mode", bytes.NewReader([]byte(`{"enabled":false}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("disable masterless mode failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/masterless/render", bytes.NewReader(render))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected render rejection when disabled: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
