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

func TestSyndicTopologyEndpoints(t *testing.T) {
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

	createNode := func(body string) {
		t.Helper()
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/control/syndic/nodes", bytes.NewReader([]byte(body)))
		s.httpServer.Handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create node failed: code=%d body=%s", rr.Code, rr.Body.String())
		}
	}
	createNode(`{"name":"master-1","role":"master","region":"us-east-1"}`)
	createNode(`{"name":"syndic-a","role":"syndic","parent":"master-1","segment":"zone-a"}`)
	createNode(`{"name":"node-1","role":"minion","parent":"syndic-a"}`)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/control/syndic/nodes", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list syndic nodes failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var nodes []map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &nodes)
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/syndic/route", bytes.NewReader([]byte(`{"target":"node-1"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("resolve syndic route failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var route struct {
		Target string   `json:"target"`
		Path   []string `json:"path"`
		Hops   int      `json:"hops"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &route)
	if route.Target != "node-1" || route.Hops != 2 || len(route.Path) != 3 {
		t.Fatalf("unexpected route response %+v", route)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/syndic/route?target=unknown", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected not found for unknown route target, got %d", rr.Code)
	}
}
