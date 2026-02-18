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
	req = httptest.NewRequest(http.MethodGet, "/v1/activity?type_prefix=queue.&contains=saturation", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("activity fetch failed: %d body=%s", rr.Code, rr.Body.String())
	}
	var activity struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &activity); err != nil {
		t.Fatalf("activity decode failed: %v", err)
	}
	found := false
	for _, evt := range activity.Items {
		if evt["type"] == "queue.saturation" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected queue.saturation event in activity stream")
	}
}

func TestActivityEndpointFiltering(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-activity.txt")+`
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
	req := httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader([]byte(`{"type":"control.audit.user","message":"user approved rollout","fields":{"actor":"alice"}}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("audit event ingest failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader([]byte(`{"type":"external.alert","message":"disk warning","fields":{"sev":"high"}}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("alert event ingest failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	since := time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/activity?since="+since+"&type_prefix=control.audit&contains=approved&limit=10&order=desc", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("activity query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Count int `json:"count"`
		Items []struct {
			Type string `json:"type"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("activity query decode failed: %v", err)
	}
	if resp.Count != 1 || len(resp.Items) != 1 || resp.Items[0].Type != "control.audit.user" {
		t.Fatalf("unexpected filtered activity result: %+v body=%s", resp, rr.Body.String())
	}
}

func TestControlHandoffEndpoint(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-handoff.txt")+`
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
	req := httptest.NewRequest(http.MethodPost, "/v1/control/queue", bytes.NewReader([]byte(`{"action":"pause"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("queue pause failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewReader([]byte(`{"config_path":"c.yaml"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected job enqueue accepted while paused, code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/emergency-stop", bytes.NewReader([]byte(`{"enabled":true,"reason":"incident"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("emergency-stop set failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/maintenance", bytes.NewReader([]byte(`{"kind":"environment","name":"prod","enabled":true,"reason":"window"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("maintenance set failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/handoff", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("handoff endpoint failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"blocked_actions"`) {
		t.Fatalf("expected blocked_actions in handoff package: %s", body)
	}
	if !strings.Contains(body, `new applies blocked by emergency stop`) {
		t.Fatalf("expected emergency-stop blocked action in handoff package: %s", body)
	}
	if !strings.Contains(body, `"active_rollouts"`) {
		t.Fatalf("expected active_rollouts in handoff package: %s", body)
	}
}

func TestControlChecklistEndpoints(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-checklist.txt")+`
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

	createBody := []byte(`{"action":"create","name":"prod migration checklist","risk_level":"high","context":{"change_id":"cr-100"}}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/checklists", bytes.NewReader(createBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("checklist create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		ID    string `json:"id"`
		Items []struct {
			ID       string `json:"id"`
			Required bool   `json:"required"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("checklist decode failed: %v", err)
	}
	if created.ID == "" || len(created.Items) == 0 {
		t.Fatalf("expected checklist id/items")
	}

	firstRequired := ""
	for _, item := range created.Items {
		if item.Required {
			firstRequired = item.ID
			break
		}
	}
	if firstRequired == "" {
		t.Fatalf("expected required checklist item")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/checklists/"+created.ID+"/complete", bytes.NewReader([]byte(`{"item_id":"`+firstRequired+`","notes":"validated"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("checklist complete failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/checklists/"+created.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("checklist get failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader([]byte(`{"entity":"checklists","mode":"human","query":"name~=migration","limit":10}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("checklists query failed: code=%d body=%s", rr.Code, rr.Body.String())
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

func TestRunbookCatalogAndLaunch(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-runbook.txt")+`
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
	req := httptest.NewRequest(http.MethodPost, "/v1/runbooks", bytes.NewReader([]byte(`{
		"name":"db emergency rollback",
		"description":"rollback db changes safely",
		"target_type":"config",
		"config_path":"c.yaml",
		"risk_level":"high",
		"owner":"sre-dba",
		"tags":["db","rollback"]
	}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("runbook create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var rb struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &rb); err != nil {
		t.Fatalf("runbook decode failed: %v", err)
	}
	if rb.ID == "" {
		t.Fatalf("expected runbook id")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/runbooks/"+rb.ID+"/launch", bytes.NewReader([]byte(`{"priority":"high"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected draft runbook launch conflict, code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/runbooks/"+rb.ID+"/approve", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("runbook approve failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/runbooks/"+rb.ID+"/launch", bytes.NewReader([]byte(`{"priority":"high"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("approved runbook launch failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/runbooks/"+rb.ID+"/deprecate", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("runbook deprecate failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader([]byte(`{"entity":"runbooks","mode":"human","query":"name~=rollback","limit":10}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("runbooks query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSolutionPackCatalogAndApply(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-solution-pack.txt")+`
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
	req := httptest.NewRequest(http.MethodGet, "/v1/solution-packs", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("solution packs list failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `stateless-vm-service`) {
		t.Fatalf("expected stateless-vm-service pack in list: %s", rr.Body.String())
	}

	out := filepath.Join(tmp, "packs", "stateless.yaml")
	body := []byte(`{"output_path":"` + out + `","create_template":true,"create_runbook":true}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/solution-packs/stateless-vm-service/apply", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("solution pack apply failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected generated solution pack config file: %v", err)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader([]byte(`{"entity":"solution_packs","mode":"human","query":"id=stateless-vm-service","limit":10}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("solution packs query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestWorkspaceTemplateCatalogAndBootstrap(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-workspace-template.txt")+`
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
	req := httptest.NewRequest(http.MethodGet, "/v1/workspace-templates?pattern=stateless-services", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("workspace templates list failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `stateless-kubernetes-service`) {
		t.Fatalf("expected stateless-kubernetes-service template in list: %s", rr.Body.String())
	}

	outDir := filepath.Join(tmp, "workspaces", "stateless")
	body := []byte(`{"output_dir":"` + outDir + `","create_template":true,"create_runbook":true}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/workspace-templates/stateless-kubernetes-service/bootstrap", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("workspace template bootstrap failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, "policy", "main.yaml")); err != nil {
		t.Fatalf("expected generated workspace policy file: %v", err)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader([]byte(`{"entity":"workspace_templates","mode":"human","query":"id=stateless-kubernetes-service","limit":10}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("workspace templates query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/workspace-templates/stateless-kubernetes-service/bootstrap", bytes.NewReader([]byte(`{"output_dir":"`+outDir+`"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected conflict when output directory exists without overwrite: code=%d body=%s", rr.Code, rr.Body.String())
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
		Results: []state.ResourceRun{
			{ResourceID: "f1", Host: "localhost", Type: "file", Changed: true},
		},
	}); err != nil {
		t.Fatalf("save run failed: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader([]byte(`{"type":"external.alert","message":"run context","fields":{"sev":"high"}}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("event ingest failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/runs/run-export-1/export", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("run export failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/runs/run-export-1/triage-bundle", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("triage bundle export failed: code=%d body=%s", rr.Code, rr.Body.String())
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
	if len(objects) < 2 {
		t.Fatalf("expected run export and triage bundle objects")
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

func TestAlertInboxEndpointDedupSuppressionAndActions(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-alerts.txt")+`
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

	eventBody := []byte(`{"type":"external.alert.disk","message":"disk full","fields":{"sev":"high","host":"db-01","service":"storage"}}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader(eventBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("first event ingest failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader(eventBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("second event ingest failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/alerts/inbox", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("alerts inbox get failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var inbox struct {
		Items []struct {
			ID          string `json:"id"`
			Fingerprint string `json:"fingerprint"`
			Count       int    `json:"count"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &inbox); err != nil {
		t.Fatalf("alerts inbox decode failed: %v", err)
	}
	if len(inbox.Items) != 1 || inbox.Items[0].Count != 2 {
		t.Fatalf("expected deduplicated alert count=2, got %+v", inbox.Items)
	}

	suppressBody := []byte(`{"action":"suppress","fingerprint":"` + inbox.Items[0].Fingerprint + `","duration_seconds":300,"reason":"maintenance"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/alerts/inbox", bytes.NewReader(suppressBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("alerts suppression failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader(eventBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("suppressed event ingest failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	ackBody := []byte(`{"action":"acknowledge","id":"` + inbox.Items[0].ID + `"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/alerts/inbox", bytes.NewReader(ackBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("alert acknowledge failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	resolveBody := []byte(`{"action":"resolve","id":"` + inbox.Items[0].ID + `"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/alerts/inbox", bytes.NewReader(resolveBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("alert resolve failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	queryBody := []byte(`{"entity":"alerts","mode":"human","query":"event_type=external.alert.disk","limit":10}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader(queryBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("alerts query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestNotificationTargetsAndDeliveries(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-notify.txt")+`
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

	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	createBody := []byte(`{"name":"tickets","kind":"ticket","url":"` + receiver.URL + `","route":"ticket","enabled":true}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/notifications/targets", bytes.NewReader(createBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("notification target create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var target struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &target); err != nil {
		t.Fatalf("notification target decode failed: %v", err)
	}
	if target.ID == "" {
		t.Fatalf("expected target id")
	}

	eventBody := []byte(`{"type":"external.alert.ticket","message":"disk full","fields":{"sev":"high","host":"db-01"}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader(eventBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("event ingest failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/notifications/deliveries", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("notification deliveries failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"delivered"`) {
		t.Fatalf("expected delivered notification in delivery log: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/notifications/targets/"+target.ID+"/disable", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("notification target disable failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestChangeRecordEndpointsLifecycle(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-change-record.txt")+`
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

	createBody := []byte(`{"summary":"db rollout","ticket_system":"jira","ticket_id":"OPS-123","ticket_url":"https://tickets.local/OPS-123","config_path":"c.yaml","requested_by":"sre-user"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/change-records", bytes.NewReader(createBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("change record create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var rec struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &rec); err != nil {
		t.Fatalf("change record decode failed: %v", err)
	}
	if rec.ID == "" {
		t.Fatalf("expected change record id")
	}

	approveBody := []byte(`{"actor":"approver-1","comment":"approved for window"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/change-records/"+rec.ID+"/approve", bytes.NewReader(approveBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("change record approve failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	jobBody := []byte(`{"config_path":"c.yaml"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewReader(jobBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("job enqueue failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var job struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &job); err != nil {
		t.Fatalf("job decode failed: %v", err)
	}

	attachBody := []byte(`{"job_id":"` + job.ID + `"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/change-records/"+rec.ID+"/attach-job", bytes.NewReader(attachBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("change record attach-job failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/change-records/"+rec.ID+"/complete", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("change record complete failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	queryBody := []byte(`{"entity":"change_records","mode":"human","query":"ticket_id=OPS-123","limit":10}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader(queryBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("change records query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSavedViewsEndpoints(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-views.txt")+`
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

	createBody := []byte(`{"name":"prod alerts","entity":"alerts","mode":"human","query":"severity=high","limit":20}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/views", bytes.NewReader(createBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("saved view create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var view struct {
		ID         string `json:"id"`
		ShareToken string `json:"share_token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &view); err != nil {
		t.Fatalf("saved view decode failed: %v", err)
	}
	if view.ID == "" || view.ShareToken == "" {
		t.Fatalf("expected saved view id/share token")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/views/"+view.ID+"/pin", bytes.NewReader([]byte(`{"pinned":true}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("saved view pin failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/views/"+view.ID+"/share", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("saved view share token refresh failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader([]byte(`{"entity":"views","mode":"human","query":"name~=alerts","limit":10}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("saved views query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/v1/views/"+view.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("saved view delete failed: code=%d body=%s", rr.Code, rr.Body.String())
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

func TestSchemaMigrationsEndpoint(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-schema.txt")+`
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
	req := httptest.NewRequest(http.MethodGet, "/v1/control/schema-migrations", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("schema migration status failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"current_version":1`) {
		t.Fatalf("expected initial schema version 1, body=%s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/schema-migrations", bytes.NewReader([]byte(`{"action":"check","from_version":1,"to_version":3}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("schema migration check failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"forward_compatible":false`) {
		t.Fatalf("expected incompatible check result, body=%s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/schema-migrations", bytes.NewReader([]byte(`{"action":"apply","from_version":1,"to_version":2,"plan_ref":"MIG-1001","notes":"add state index"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("schema migration apply failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/schema-migrations", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("schema migration status (after apply) failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"current_version":2`) {
		t.Fatalf("expected updated schema version 2, body=%s", rr.Body.String())
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

func TestRunDigestEndpoint(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-digest.txt")+`
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

	st := state.New(tmp)
	if err := st.SaveRun(state.RunRecord{
		ID:        "digest-success-1",
		StartedAt: time.Now().UTC().Add(-5 * time.Minute),
		EndedAt:   time.Now().UTC().Add(-4 * time.Minute),
		Status:    state.RunSucceeded,
		Results:   []state.ResourceRun{{ResourceID: "r1", Changed: true}},
	}); err != nil {
		t.Fatalf("save success run failed: %v", err)
	}
	if err := st.SaveRun(state.RunRecord{
		ID:        "digest-fail-1",
		StartedAt: time.Now().UTC().Add(-3 * time.Minute),
		EndedAt:   time.Now().UTC().Add(-2 * time.Minute),
		Status:    state.RunFailed,
		Results:   []state.ResourceRun{{ResourceID: "r2", Changed: true}},
	}); err != nil {
		t.Fatalf("save failed run failed: %v", err)
	}

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/digest?hours=1", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("run digest failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"total_runs":2`) {
		t.Fatalf("expected total_runs=2, body=%s", body)
	}
	if !strings.Contains(body, `"failed_runs":1`) {
		t.Fatalf("expected failed_runs=1, body=%s", body)
	}
	if !strings.Contains(body, `"latent_risk_score"`) {
		t.Fatalf("expected latent risk score in digest, body=%s", body)
	}
	if !strings.Contains(body, `digest-fail-1`) {
		t.Fatalf("expected failed run id in digest output, body=%s", body)
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

func TestUpgradeAssistantEndpoint(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x18.txt")+`
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
	req := httptest.NewRequest(http.MethodGet, "/v1/release/upgrade-assistant", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upgrade assistant get failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"remove_after_version":"v3"`) {
		t.Fatalf("expected deprecation guidance in get response: %s", rr.Body.String())
	}

	conflictBody := []byte(`{"baseline":{"version":"v1","endpoints":["GET /healthz","GET /v1/legacy-endpoint"]}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/upgrade-assistant", bytes.NewReader(conflictBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected upgrade assistant conflict for breaking removal, code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"severity":"error"`) {
		t.Fatalf("expected error advice in conflict response: %s", rr.Body.String())
	}
}
