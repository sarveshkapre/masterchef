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

func TestRunbookCatalogEndpoint(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	features := filepath.Join(tmp, "features.md")
	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: marker
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "x-runbook-catalog.txt")+`
    content: "ok"
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
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	createDB := []byte(`{"name":"db-runbook","target_type":"config","config_path":"c.yaml","risk_level":"high","owner":"db-team","tags":["database"]}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/runbooks", bytes.NewReader(createDB))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create db runbook failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var db struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &db); err != nil {
		t.Fatalf("decode db runbook failed: %v", err)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/runbooks/"+db.ID+"/approve", nil)
	rr = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("approve db runbook failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	createWeb := []byte(`{"name":"web-runbook","target_type":"config","config_path":"c.yaml","risk_level":"low","owner":"web-team","tags":["web"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/runbooks", bytes.NewReader(createWeb))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create web runbook failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var web struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &web); err != nil {
		t.Fatalf("decode web runbook failed: %v", err)
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/runbooks/"+web.ID+"/approve", nil)
	rr = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("approve web runbook failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/runbooks/catalog?owner=web-team&max_risk_level=medium", nil)
	rr = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("runbook catalog by owner failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"name":"web-runbook"`)) {
		t.Fatalf("expected web runbook in catalog response: %s", rr.Body.String())
	}
	if bytes.Contains(rr.Body.Bytes(), []byte(`"name":"db-runbook"`)) {
		t.Fatalf("did not expect high-risk db runbook in filtered catalog: %s", rr.Body.String())
	}
}
