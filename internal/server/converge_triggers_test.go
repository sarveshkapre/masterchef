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

func TestConvergeTriggerEndpoints(t *testing.T) {
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
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })

	create := []byte(`{"source":"policy","event_type":"policy.updated","event_id":"evt-1","config_path":"c.yaml","priority":"high","idempotency_key":"trigger-1","payload":{"bundle":"prod"}}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/converge/triggers", bytes.NewReader(create))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("create converge trigger failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		JobID  string `json:"job_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created trigger failed: %v", err)
	}
	if created.ID == "" || created.Status != "queued" || created.JobID == "" {
		t.Fatalf("unexpected created trigger: %+v", created)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/converge/triggers?limit=10", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list converge triggers failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), created.ID) {
		t.Fatalf("expected created trigger in list: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/converge/triggers/"+created.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get converge trigger failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	recordOnly := []byte(`{"source":"security","event_type":"cve.critical","config_path":"c.yaml","auto_enqueue":false}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/converge/triggers", bytes.NewReader(recordOnly))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted || !strings.Contains(rr.Body.String(), `"status":"recorded"`) {
		t.Fatalf("record-only trigger failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	freeze := []byte(`{"enabled":true,"duration_seconds":120,"reason":"window"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/freeze", bytes.NewReader(freeze))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("enable freeze failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	blocked := []byte(`{"source":"package","event_type":"package.updated","config_path":"c.yaml","priority":"high"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/converge/triggers", bytes.NewReader(blocked))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected blocked trigger during freeze: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"blocked"`) {
		t.Fatalf("expected blocked trigger status in response: %s", rr.Body.String())
	}
}
