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

func TestExportedResourceEndpoints(t *testing.T) {
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
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })

	createA := []byte(`{"type":"service","host":"node-a","resource_id":"svc-db","source":"agent","attributes":{"role":"db","env":"prod"}}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/resources/exported", bytes.NewReader(createA))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create exported resource A failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	createB := []byte(`{"type":"service","host":"node-b","resource_id":"svc-web","source":"agent","attributes":{"role":"web","env":"prod"}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/resources/exported", bytes.NewReader(createB))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create exported resource B failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/resources/exported?limit=10", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list exported resources failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"resource_id":"svc-db"`) || !strings.Contains(rr.Body.String(), `"resource_id":"svc-web"`) {
		t.Fatalf("expected both exported resources in list: %s", rr.Body.String())
	}

	collect := []byte(`{"selector":"type=service and attrs.role=db","limit":5}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/resources/collect", bytes.NewReader(collect))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("collect exported resources failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"count":1`) || !strings.Contains(rr.Body.String(), `"resource_id":"svc-db"`) {
		t.Fatalf("unexpected collector response: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"resource_id":"svc-web"`) {
		t.Fatalf("collector should not include svc-web: %s", rr.Body.String())
	}

	badCollect := []byte(`{"selector":"type","limit":5}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/resources/collect", bytes.NewReader(badCollect))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid selector error: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
