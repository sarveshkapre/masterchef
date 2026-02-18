package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/state"
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

func TestFreezeEndpointBlocksAppliesUnlessForced(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x4.txt")+`
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
	req := httptest.NewRequest(http.MethodPost, "/v1/control/freeze", bytes.NewReader([]byte(`{"enabled":true,"duration_seconds":120,"reason":"window"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("freeze enable failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewReader([]byte(`{"config_path":"c.yaml"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected freeze to block apply: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewReader([]byte(`{"config_path":"c.yaml"}`)))
	req.Header.Set("X-Force-Apply", "true")
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected force apply to bypass freeze: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestWorkflowEndpoints(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x5.txt")+`
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

	makeTemplate := func(name string) string {
		body := []byte(`{"name":"` + name + `","config_path":"c.yaml"}`)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/templates", bytes.NewReader(body))
		s.httpServer.Handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("template create failed: code=%d body=%s", rr.Code, rr.Body.String())
		}
		var tpl struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &tpl); err != nil {
			t.Fatalf("template decode failed: %v", err)
		}
		return tpl.ID
	}

	step1 := makeTemplate("wf-step-1")
	step2 := makeTemplate("wf-step-2")

	wfBody := []byte(`{
		"name":"pipeline",
		"steps":[
			{"template_id":"` + step1 + `","priority":"high"},
			{"template_id":"` + step2 + `","priority":"normal"}
		]
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/workflows", bytes.NewReader(wfBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("workflow create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var wf struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &wf); err != nil {
		t.Fatalf("workflow decode failed: %v", err)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/workflows/"+wf.ID+"/launch", bytes.NewReader([]byte(`{"priority":"normal"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("workflow launch failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var run struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &run); err != nil {
		t.Fatalf("workflow run decode failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/v1/workflow-runs/"+run.ID, nil)
		s.httpServer.Handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("workflow run get failed: code=%d body=%s", rr.Code, rr.Body.String())
		}
		var cur struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &cur); err != nil {
			t.Fatalf("workflow run status decode failed: %v", err)
		}
		if cur.Status == "succeeded" {
			break
		}
		if cur.Status == "failed" {
			t.Fatalf("workflow run failed unexpectedly: %s", rr.Body.String())
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for workflow run completion")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCommandIngestWithChecksumAndDeadLetters(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x6.txt")+`
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

	badBody := []byte(`{"action":"apply","config_path":"c.yaml","priority":"high","checksum":"bad"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/commands/ingest", bytes.NewReader(badBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected checksum mismatch dead letter: code=%d body=%s", rr.Code, rr.Body.String())
	}

	checksum := control.ComputeCommandChecksum("apply", "c.yaml", "high", "cmd-1")
	goodBody := []byte(`{"action":"apply","config_path":"c.yaml","priority":"high","idempotency_key":"cmd-1","checksum":"` + checksum + `"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/commands/ingest", bytes.NewReader(goodBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected accepted command ingest: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/commands/dead-letters", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected dead-letter listing success: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var dead []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &dead); err != nil {
		t.Fatalf("dead-letter decode failed: %v", err)
	}
	if len(dead) == 0 {
		t.Fatalf("expected at least one dead letter")
	}
}

func TestAssociationEndpoints(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x7.txt")+`
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
		"config_path":"c.yaml",
		"target_kind":"environment",
		"target_name":"prod",
		"interval_seconds":1,
		"priority":"normal",
		"enabled":true
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/associations", bytes.NewReader(createBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("association create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var assoc struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &assoc); err != nil {
		t.Fatalf("association decode failed: %v", err)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/associations", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("association list failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/associations/"+assoc.ID+"/disable", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("association disable failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/associations/"+assoc.ID+"/revisions", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("association revisions failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/associations/"+assoc.ID+"/replay", bytes.NewReader([]byte(`{"revision":1}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("association replay failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestQueryEndpointHumanAndASTModes(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x8.txt")+`
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
	req := httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader([]byte(`{"type":"external.alert","message":"monitor","fields":{"env":"prod","sev":"high"}}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("event ingest failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewReader([]byte(`{"config_path":"c.yaml","priority":"high"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("job enqueue failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	humanQuery := []byte(`{"entity":"events","mode":"human","query":"type=external.alert AND fields.env=prod","limit":20}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader(humanQuery))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("human query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var humanResp struct {
		MatchedCount int `json:"matched_count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &humanResp); err != nil {
		t.Fatalf("human query decode failed: %v", err)
	}
	if humanResp.MatchedCount < 1 {
		t.Fatalf("expected human query to match at least one event")
	}

	astQuery := []byte(`{
		"entity":"jobs",
		"mode":"ast",
		"query_ast":{"field":"priority","comparator":"eq","value":"high"},
		"limit":20
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader(astQuery))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ast query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var astResp struct {
		MatchedCount int `json:"matched_count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &astResp); err != nil {
		t.Fatalf("ast query decode failed: %v", err)
	}
	if astResp.MatchedCount < 1 {
		t.Fatalf("expected ast query to match at least one job")
	}
}

func TestObjectStoreRunAndAssociationExport(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x9.txt")+`
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

	st := state.New(tmp)
	if err := st.SaveRun(state.RunRecord{
		ID:        "run-export-1",
		StartedAt: time.Now().UTC().Add(-time.Second),
		EndedAt:   time.Now().UTC(),
		Status:    state.RunSucceeded,
	}); err != nil {
		t.Fatalf("save run failed: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/run-export-1/export", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("run export failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	createBody := []byte(`{
		"config_path":"c.yaml",
		"target_kind":"environment",
		"target_name":"prod",
		"interval_seconds":1,
		"priority":"normal",
		"enabled":true
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/associations", bytes.NewReader(createBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("association create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var assoc struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &assoc); err != nil {
		t.Fatalf("association decode failed: %v", err)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/associations/"+assoc.ID+"/export", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("association export failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/object-store/objects?prefix=runs/run-export-1", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("object list failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var objects []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &objects); err != nil {
		t.Fatalf("object list decode failed: %v", err)
	}
	if len(objects) < 1 {
		t.Fatalf("expected at least one exported object")
	}
}

func TestCanaryEndpointsAndHealthSummary(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x10.txt")+`
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

	createBody := []byte(`{"name":"control-plane-canary","config_path":"c.yaml","priority":"high","interval_seconds":1,"failure_threshold":2}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/canaries", bytes.NewReader(createBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("canary create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var canary struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &canary); err != nil {
		t.Fatalf("canary decode failed: %v", err)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/canaries", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("canary list failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/canaries/"+canary.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("canary get failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/canaries/"+canary.ID+"/disable", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("canary disable failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/canary-health", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("canary health failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var health struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &health); err != nil {
		t.Fatalf("canary health decode failed: %v", err)
	}
	if health.Total < 1 {
		t.Fatalf("expected at least one registered canary")
	}
}

func TestPreflightEndpoint(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x11.txt")+`
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

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	passBody := []byte(`{"tcp":["` + ln.Addr().String() + `"],"storage_paths":["` + tmp + `"],"require_object_store":true,"require_queue":true}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/preflight", bytes.NewReader(passBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected preflight pass: code=%d body=%s", rr.Code, rr.Body.String())
	}

	failBody := []byte(`{"dns":["invalid.invalid.masterchef.local"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/preflight", bytes.NewReader(failBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected preflight fail status: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRulebookEventPipeline(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x12.txt")+`
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

	ruleBody := []byte(`{
		"name":"critical alert remediation",
		"source_prefix":"external.alert",
		"match_mode":"all",
		"conditions":[{"field":"fields.sev","comparator":"eq","value":"critical"}],
		"actions":[{"type":"enqueue_apply","config_path":"c.yaml","priority":"high"}]
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/rules", bytes.NewReader(ruleBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("rule create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var createdRule struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &createdRule); err != nil {
		t.Fatalf("rule decode failed: %v", err)
	}
	if createdRule.ID == "" {
		t.Fatalf("expected created rule id")
	}

	eventBody := []byte(`{"type":"external.alert","message":"disk full","fields":{"sev":"critical"}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader(eventBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("event ingest failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
		s.httpServer.Handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("jobs list failed: code=%d body=%s", rr.Code, rr.Body.String())
		}
		var jobs []map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &jobs); err != nil {
			t.Fatalf("jobs decode failed: %v", err)
		}
		found := false
		for _, job := range jobs {
			if priority, _ := job["priority"].(string); priority == "high" {
				found = true
				break
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for rule-triggered high-priority job")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestReleaseReadinessEndpoint(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x14.txt")+`
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
	req := httptest.NewRequest(http.MethodGet, "/v1/release/readiness", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("threshold get failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	failBody := []byte(`{"signals":{"quality_score":0.5,"reliability_score":0.5,"performance_score":0.5,"test_pass_rate":0.5,"flake_rate":0.5,"open_critical_incidents":1,"p95_apply_latency_ms":999999}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/readiness", bytes.NewReader(failBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected readiness block (409), got code=%d body=%s", rr.Code, rr.Body.String())
	}

	passBody := []byte(`{"signals":{"quality_score":0.95,"reliability_score":0.95,"performance_score":0.95,"test_pass_rate":0.99,"flake_rate":0.01,"open_critical_incidents":0,"p95_apply_latency_ms":1000}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/readiness", bytes.NewReader(passBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected readiness pass (200), got code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestWebhookEndpointsAndDeliveries(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x13.txt")+`
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

	var calls int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	webhookBody := []byte(`{"name":"alerts","url":"` + receiver.URL + `","event_prefix":"external.alert","enabled":true}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks", bytes.NewReader(webhookBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("webhook create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var webhook struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &webhook); err != nil {
		t.Fatalf("webhook decode failed: %v", err)
	}

	eventBody := []byte(`{"type":"external.alert.disk","message":"disk alert","fields":{"sev":"high"}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader(eventBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("event ingest failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if atomic.LoadInt32(&calls) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for webhook delivery")
		}
		time.Sleep(10 * time.Millisecond)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/webhooks/deliveries?limit=10", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("webhook deliveries failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var deliveries []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &deliveries); err != nil {
		t.Fatalf("deliveries decode failed: %v", err)
	}
	if len(deliveries) < 1 {
		t.Fatalf("expected at least one delivery record")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/webhooks/"+webhook.ID+"/disable", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("webhook disable failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestChannelsEndpointAndCompatibility(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x15.txt")+`
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
	req := httptest.NewRequest(http.MethodPost, "/v1/control/channels", bytes.NewReader([]byte(`{"action":"set_channel","component":"agent","channel":"candidate"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set channel failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/channels", bytes.NewReader([]byte(`{"action":"set_channel","component":"agent","channel":"lts"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set lts channel failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/channels?control_plane_protocol=5", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get channels failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/channels", bytes.NewReader([]byte(`{"action":"check_compatibility","control_plane_protocol":5,"agent_protocol":4}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected compatibility pass: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/channels", bytes.NewReader([]byte(`{"action":"check_compatibility","control_plane_protocol":5,"agent_protocol":3}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected compatibility conflict: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/channels", bytes.NewReader([]byte(`{"action":"support_matrix","control_plane_protocol":5}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected support matrix endpoint to pass: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"channel":"lts"`) {
		t.Fatalf("expected lts channel in support matrix: %s", rr.Body.String())
	}
}

func TestBackupRestoreEndpoints(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x16.txt")+`
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

	st := state.New(tmp)
	if err := st.SaveRun(state.RunRecord{
		ID:        "backup-run-1",
		StartedAt: time.Now().UTC().Add(-time.Second),
		EndedAt:   time.Now().UTC(),
		Status:    state.RunSucceeded,
	}); err != nil {
		t.Fatalf("save run failed: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader([]byte(`{"type":"external.alert","message":"from monitor","fields":{"sev":"high"}}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("event ingest failed: %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/backup", bytes.NewReader([]byte(`{"include_runs":true,"include_events":true}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("backup failed: %d body=%s", rr.Code, rr.Body.String())
	}
	var backupResp struct {
		Object struct {
			Key       string    `json:"key"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"object"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &backupResp); err != nil {
		t.Fatalf("backup decode failed: %v", err)
	}
	if backupResp.Object.Key == "" {
		t.Fatalf("expected backup object key")
	}
	if backupResp.Object.CreatedAt.IsZero() {
		t.Fatalf("expected backup object created_at")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/backups", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("backups list failed: %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/drill", bytes.NewReader([]byte(`{"include_runs":true,"include_events":true}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("drill failed: %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"verified"`) {
		t.Fatalf("expected drill verification status, body=%s", rr.Body.String())
	}

	if err := st.SaveRun(state.RunRecord{
		ID:        "backup-run-2",
		StartedAt: time.Now().UTC().Add(-500 * time.Millisecond),
		EndedAt:   time.Now().UTC(),
		Status:    state.RunSucceeded,
	}); err != nil {
		t.Fatalf("save second run failed: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/backup", bytes.NewReader([]byte(`{"include_runs":true,"include_events":true}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("second backup failed: %d body=%s", rr.Code, rr.Body.String())
	}
	var backupResp2 struct {
		Object struct {
			Key string `json:"key"`
		} `json:"object"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &backupResp2); err != nil {
		t.Fatalf("second backup decode failed: %v", err)
	}
	if backupResp2.Object.Key == "" {
		t.Fatalf("expected second backup key")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/restore", bytes.NewReader([]byte(`{"key":"`+backupResp.Object.Key+`","verify_only":true}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("restore verify failed: %d body=%s", rr.Code, rr.Body.String())
	}

	if err := st.ReplaceRuns([]state.RunRecord{}); err != nil {
		t.Fatalf("clear runs failed: %v", err)
	}

	rr = httptest.NewRecorder()
	restoreAt := backupResp.Object.CreatedAt.Format(time.RFC3339Nano)
	req = httptest.NewRequest(http.MethodPost, "/v1/control/restore", bytes.NewReader([]byte(`{"prefix":"backups","at_or_before":"`+restoreAt+`"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("point-in-time restore failed: %d body=%s", rr.Code, rr.Body.String())
	}

	runs, err := st.ListRuns(10)
	if err != nil {
		t.Fatalf("list runs after restore failed: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "backup-run-1" {
		t.Fatalf("expected point-in-time restore to recover first backup state, got %+v", runs)
	}

	if err := st.ReplaceRuns([]state.RunRecord{}); err != nil {
		t.Fatalf("clear runs failed: %v", err)
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/restore", bytes.NewReader([]byte(`{"key":"`+backupResp2.Object.Key+`"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("restore by key failed: %d body=%s", rr.Code, rr.Body.String())
	}
	runs, err = st.ListRuns(10)
	if err != nil {
		t.Fatalf("list runs after restore-by-key failed: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected latest restore-by-key to recover two runs, got %+v", runs)
	}
}

func TestAPIContractEndpoint(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x17.txt")+`
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
	req := httptest.NewRequest(http.MethodGet, "/v1/release/api-contract", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("api contract get failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	passBody := []byte(`{"baseline":{"version":"v0","endpoints":["GET /healthz"]}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/api-contract", bytes.NewReader(passBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("api contract diff pass failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	conflictBody := []byte(`{"baseline":{"version":"v0","endpoints":["GET /healthz","GET /missing-endpoint"]}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/api-contract", bytes.NewReader(conflictBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected backward-compat conflict, got code=%d body=%s", rr.Code, rr.Body.String())
	}

	lifecycleConflictBody := []byte(`{"baseline":{"version":"v1","endpoints":["GET /healthz","GET /v1/legacy-endpoint"],"deprecations":[{"endpoint":"GET /v1/legacy-endpoint","announced_version":"v1","remove_after_version":"v3"}]}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/api-contract", bytes.NewReader(lifecycleConflictBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected deprecation lifecycle conflict for early removal, got code=%d body=%s", rr.Code, rr.Body.String())
	}

	lifecyclePassBody := []byte(`{"baseline":{"version":"v1","endpoints":["GET /healthz","GET /v1/legacy-endpoint"],"deprecations":[{"endpoint":"GET /v1/legacy-endpoint","announced_version":"v1","remove_after_version":"v1"}]}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/api-contract", bytes.NewReader(lifecyclePassBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected lifecycle-compliant removal to pass, got code=%d body=%s", rr.Code, rr.Body.String())
	}
}
