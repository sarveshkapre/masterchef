package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInventoryCMDBImportEndpoint(t *testing.T) {
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

	body := []byte(`{
  "source_system":"servicenow",
  "records":[
    {"name":"cmdb-node-1","address":"10.20.0.1","transport":"ssh","roles":["app"]},
    {"name":"cmdb-node-2","address":"10.20.0.2","transport":"winrm","roles":["db"]}
  ]
}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/inventory/import/cmdb", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("cmdb import failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"imported":2`) {
		t.Fatalf("expected imported count in response: %s", rr.Body.String())
	}

	dryRun := []byte(`{"source_system":"servicenow","dry_run":true,"records":[{"name":"cmdb-node-1"}]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/inventory/import/cmdb", bytes.NewReader(dryRun))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("cmdb import dry-run failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"would_update"`) {
		t.Fatalf("expected would_update status in dry-run response: %s", rr.Body.String())
	}

	assistant := []byte(`{"type":"role_hierarchy","source_system":"puppetdb","sample_fields":["group","env_name"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/inventory/import/assist", bytes.NewReader(assistant))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("import assistant failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"required_fields"`) {
		t.Fatalf("expected assistant response details: %s", rr.Body.String())
	}
}
