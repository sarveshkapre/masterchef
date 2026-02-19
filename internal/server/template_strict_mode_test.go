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

func TestTemplateStrictModeRenderAndLaunch(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "template-config.yaml")
	features := filepath.Join(tmp, "features.md")
	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: env-file
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "rendered.txt")+`
    content: "environment={{env}}\n"
`), 0o644); err != nil {
		t.Fatal(err)
	}
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
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })

	createBody := []byte(`{"name":"strict-template","config_path":"` + cfg + `","strict_mode":true}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/templates", bytes.NewReader(createBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("template create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var tpl struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &tpl); err != nil {
		t.Fatalf("decode template failed: %v", err)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/templates/"+tpl.ID+"/render", bytes.NewReader([]byte(`{"answers":{}}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected strict render to fail without vars: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/templates/"+tpl.ID+"/launch", bytes.NewReader([]byte(`{"answers":{}}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected strict launch to fail without vars: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/templates/"+tpl.ID+"/render", bytes.NewReader([]byte(`{"answers":{"env":"prod"}}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected strict render with vars to succeed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/templates/"+tpl.ID+"/launch", bytes.NewReader([]byte(`{"answers":{"env":"prod"}}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected strict launch with vars to succeed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"missing_variables":[]`)) {
		t.Fatalf("expected no missing variables on strict launch, body=%s", rr.Body.String())
	}
}
