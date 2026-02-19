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

func TestNativeSchedulerEndpoints(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/v1/execution/native-schedulers", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list native schedulers failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"name":"systemd_timer"`) {
		t.Fatalf("expected systemd scheduler backend: %s", rr.Body.String())
	}

	selectBody := []byte(`{"os_family":"linux","interval_seconds":30,"jitter_seconds":5}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/native-schedulers/select", bytes.NewReader(selectBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("select native scheduler failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"name":"systemd_timer"`) {
		t.Fatalf("expected systemd selection: %s", rr.Body.String())
	}

	unsupportedPreferred := []byte(`{"os_family":"linux","interval_seconds":30,"jitter_seconds":5,"preferred_backend":"cron"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/native-schedulers/select", bytes.NewReader(unsupportedPreferred))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected conflict for unsupported preferred backend: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAssociationCreateSetsSchedulerBackend(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "cfg.yaml")
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

	create := []byte(`{"config_path":"cfg.yaml","target_kind":"environment","target_name":"prod","interval_seconds":1,"enabled":true}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/associations", bytes.NewReader(create))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create association failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var assoc struct {
		Backend string `json:"scheduler_backend"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &assoc); err != nil {
		t.Fatalf("decode association response: %v", err)
	}
	if assoc.Backend == "" {
		t.Fatalf("expected scheduler backend to be set")
	}
	if assoc.Backend != "embedded_scheduler" {
		t.Fatalf("expected embedded scheduler for 1s interval, got %q", assoc.Backend)
	}
}
