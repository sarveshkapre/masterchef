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

	"github.com/masterchef/masterchef/internal/control"
)

func TestInventoryVariableBundleExportImportEndpoints(t *testing.T) {
	sourceDir := t.TempDir()
	features := filepath.Join(sourceDir, "features.md")
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

	source := New(":0", sourceDir)
	t.Cleanup(func() {
		_ = source.Shutdown(context.Background())
	})

	enrollBody := []byte(`{"name":"port-node-1","address":"10.40.0.1","transport":"ssh","source":"cmdb-sync"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/inventory/enroll", bytes.NewReader(enrollBody))
	source.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("enroll failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/inventory/runtime-hosts/port-node-1/activate", bytes.NewReader([]byte(`{"reason":"ready"}`)))
	source.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("activate failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	roleBody := []byte(`{"name":"web","run_list":["recipe[web]"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/roles", bytes.NewReader(roleBody))
	source.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("role create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	envBody := []byte(`{"name":"prod","override_attributes":{"tier":"critical"}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/environments", bytes.NewReader(envBody))
	source.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("environment create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	secretBody := []byte(`{"name":"prod-vars","data":{"api_key":"xyz"},"passphrase":"bundle-pass"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/vars/encrypted/files", bytes.NewReader(secretBody))
	source.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("encrypted var create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/inventory/export/bundle", bytes.NewReader([]byte(`{}`)))
	source.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("bundle export failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var bundle control.InventoryVariableBundle
	if err := json.Unmarshal(rr.Body.Bytes(), &bundle); err != nil {
		t.Fatalf("decode export bundle failed: %v", err)
	}
	if len(bundle.Inventory) != 1 || len(bundle.Roles) != 1 || len(bundle.Environments) != 1 || len(bundle.EncryptedVariableFiles) != 1 {
		t.Fatalf("unexpected export bundle contents: %+v", bundle)
	}

	targetDir := t.TempDir()
	targetFeatures := filepath.Join(targetDir, "features.md")
	if err := os.WriteFile(targetFeatures, []byte(`# Features
- foo
## Competitor Feature Traceability Matrix (Strict 1:1)
### Chef -> Masterchef
| ID | Chef Feature | Masterchef 1:1 Mapping |
|---|---|---|
| CHEF-1 | X | foo |
`), 0o644); err != nil {
		t.Fatal(err)
	}
	target := New(":0", targetDir)
	t.Cleanup(func() {
		_ = target.Shutdown(context.Background())
	})

	importBody, err := json.Marshal(control.InventoryVariableImportRequest{Bundle: bundle})
	if err != nil {
		t.Fatalf("encode import request failed: %v", err)
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/inventory/import/bundle", bytes.NewReader(importBody))
	target.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("bundle import failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	var importResp control.InventoryVariableImportResult
	if err := json.Unmarshal(rr.Body.Bytes(), &importResp); err != nil {
		t.Fatalf("decode import response failed: %v", err)
	}
	if importResp.Inventory.Imported != 1 || importResp.Roles.Imported != 1 || importResp.Environments.Imported != 1 || importResp.EncryptedVariableFiles.Imported != 1 {
		t.Fatalf("unexpected import counts: %+v", importResp)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/inventory/runtime-hosts/port-node-1", nil)
	target.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected imported runtime host: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/roles/web", nil)
	target.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected imported role: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/environments/prod", nil)
	target.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected imported environment: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/vars/encrypted/files/prod-vars?passphrase=bundle-pass", nil)
	target.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected imported encrypted var to decrypt: code=%d body=%s", rr.Code, rr.Body.String())
	}

	dryRunBody, err := json.Marshal(control.InventoryVariableImportRequest{DryRun: true, Bundle: bundle})
	if err != nil {
		t.Fatalf("encode dry-run request failed: %v", err)
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/inventory/import/bundle", bytes.NewReader(dryRunBody))
	target.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("bundle import dry-run failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
