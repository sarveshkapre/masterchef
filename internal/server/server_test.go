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

func TestSplitPath(t *testing.T) {
	got := splitPath("/v1/schedules/abc/disable")
	if len(got) != 4 || got[0] != "v1" || got[2] != "abc" || got[3] != "disable" {
		t.Fatalf("unexpected split: %#v", got)
	}
}

func TestJobsAndSchedulesEndpoints(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	features := filepath.Join(tmp, "features.md")

	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: f1
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "x.txt")+`
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

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("health status code: got=%d", rr.Code)
	}
	if rr.Header().Get("X-Request-ID") == "" {
		t.Fatalf("expected request ID header")
	}

	body := []byte(`{"config_path":"c.yaml","interval_seconds":1}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/schedules", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("schedule create status code: got=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/metrics", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("metrics status code: got=%d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/runs", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("runs status code: got=%d", rr.Code)
	}

	body = []byte(`{"name":"demo","config_path":"c.yaml"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/templates", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("template create status code: got=%d body=%s", rr.Code, rr.Body.String())
	}

	body = []byte(`{"enabled":true,"reason":"incident"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/emergency-stop", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("emergency stop status code: got=%d body=%s", rr.Code, rr.Body.String())
	}

	body = []byte(`{"config_path":"c.yaml"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected conflict when emergency stop enabled: got=%d body=%s", rr.Code, rr.Body.String())
	}

	body = []byte(`{"action":"pause"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/queue", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("queue pause status code: got=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/queue", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("queue status code: got=%d body=%s", rr.Code, rr.Body.String())
	}

	body = []byte(`{"kind":"environment","name":"prod","enabled":true,"reason":"window"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/maintenance", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("maintenance set status code: got=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/maintenance", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("maintenance list status code: got=%d body=%s", rr.Code, rr.Body.String())
	}

	body = []byte(`{"action":"set_capacity","max_backlog":200,"max_execution_cost":20}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/capacity", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("capacity set status code: got=%d body=%s", rr.Code, rr.Body.String())
	}

	body = []byte(`{"action":"set_host_health","host":"db-01","healthy":false}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/capacity", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("capacity host health status code: got=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/capacity", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("capacity get status code: got=%d body=%s", rr.Code, rr.Body.String())
	}

	body = []byte(`{"max_age_seconds":1}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/recover-stuck", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("recover-stuck status code: got=%d body=%s", rr.Code, rr.Body.String())
	}

	body = []byte(`{"type":"external.alert","message":"from monitor","fields":{"sev":"high"}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("event ingest status code: got=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestQueuePriorityAndSaturationSignals(t *testing.T) {
	t.Setenv("MC_QUEUE_BACKLOG_SLO_THRESHOLD", "1")

	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	features := filepath.Join(tmp, "features.md")

	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: f1
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "x2.txt")+`
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

	pauseBody := []byte(`{"action":"pause"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/queue", bytes.NewReader(pauseBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("queue pause failed: %d body=%s", rr.Code, rr.Body.String())
	}

	priorities := make([]string, 0, 2)
	for _, body := range [][]byte{
		[]byte(`{"config_path":"c.yaml","priority":"high"}`),
		[]byte(`{"config_path":"c.yaml","priority":"low"}`),
	} {
		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewReader(body))
		s.httpServer.Handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusAccepted {
			t.Fatalf("job create failed: %d body=%s", rr.Code, rr.Body.String())
		}
		var jobResp struct {
			Priority string `json:"priority"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &jobResp); err != nil {
			t.Fatalf("job decode failed: %v", err)
		}
		priorities = append(priorities, jobResp.Priority)
	}
	if len(priorities) != 2 || priorities[0] != "high" || priorities[1] != "low" {
		t.Fatalf("unexpected response priorities: %#v", priorities)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/activity", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("activity fetch failed: %d body=%s", rr.Code, rr.Body.String())
	}
	var events []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &events); err != nil {
		t.Fatalf("activity decode failed: %v", err)
	}
	found := false
	for _, evt := range events {
		if evt["type"] == "queue.saturation" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected queue.saturation event in activity stream")
	}
}

func TestTemplateLaunchSurveyValidation(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	features := filepath.Join(tmp, "features.md")

	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: f1
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "x3.txt")+`
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

	createBody := []byte(`{
		"name":"deploy",
		"config_path":"c.yaml",
		"survey":{
			"env":{"type":"string","required":true,"enum":["prod","staging"]},
			"retries":{"type":"int"}
		}
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/templates", bytes.NewReader(createBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("template create failed: %d body=%s", rr.Code, rr.Body.String())
	}
	var tpl struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &tpl); err != nil {
		t.Fatalf("template decode failed: %v", err)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/templates/"+tpl.ID+"/launch", bytes.NewReader([]byte(`{"answers":{"retries":"3"}}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected validation error for missing required survey field: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/templates/"+tpl.ID+"/launch", bytes.NewReader([]byte(`{"answers":{"env":"prod","retries":"3"}}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected successful launch: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
