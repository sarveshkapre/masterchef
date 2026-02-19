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

func TestProxyMinionEndpoints(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "proxy.yaml")
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
    path: `+filepath.Join(tmp, "marker.txt")+`
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

	bindBody := []byte(`{"proxy_id":"proxy-east-1","device_id":"switch-1","transport":"netconf","metadata":{"site":"dc1"}}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/proxy-minions", bytes.NewReader(bindBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create proxy-minion binding failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var binding struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &binding)
	if binding.ID == "" {
		t.Fatalf("expected binding id")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/agents/proxy-minions/"+binding.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get binding failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	dispatchBody := []byte(`{"device_id":"switch-1","config_path":"proxy.yaml","priority":"high"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/proxy-minions/dispatch", bytes.NewReader(dispatchBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("proxy-minion dispatch failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var rec struct {
		ID    string `json:"id"`
		JobID string `json:"job_id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &rec)
	if rec.ID == "" || rec.JobID == "" {
		t.Fatalf("expected dispatch record and job id, got %+v", rec)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/agents/proxy-minions/dispatch?limit=10", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list proxy dispatches failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
