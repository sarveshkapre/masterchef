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
	"strconv"
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

func TestMigrationAssessmentEndpoints(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-migration.txt")+`
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

	assessBody := []byte(`{
  "source_platform":"chef",
  "workload":"payments",
  "used_features":["recipes","data bags","ruby block"],
  "semantic_checks":[{"name":"idempotency","expected":"no-op","translated":"changes each run"}],
  "deprecations":[{"name":"legacy-chef-handler","severity":"high","eol_date":"2026-03-01","replacement":"event hooks"}]
}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/migrations/assess", bytes.NewReader(assessBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("migration assess failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	type assessResp struct {
		ID          string `json:"id"`
		ParityScore int    `json:"parity_score"`
		RiskScore   int    `json:"risk_score"`
	}
	var created assessResp
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode assess response failed: %v body=%s", err, rr.Body.String())
	}
	if created.ID == "" {
		t.Fatalf("expected migration report id in assess response")
	}
	if created.ParityScore <= 0 || created.RiskScore <= 0 {
		t.Fatalf("unexpected scores in assess response: %+v", created)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/migrations/reports", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("migration reports list failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), created.ID) {
		t.Fatalf("expected created migration report in list: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/migrations/reports/"+created.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("migration report by id failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader([]byte(`{"entity":"migration_reports","mode":"human","query":"source_platform=chef","limit":10}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("migration reports query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestUseCaseTemplateCatalogAndApply(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-use-case-template.txt")+`
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
	req := httptest.NewRequest(http.MethodGet, "/v1/use-case-templates?scenario=release-rollout", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("use-case templates list failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `blue-green-release`) {
		t.Fatalf("expected blue-green-release template in list: %s", rr.Body.String())
	}

	outDir := filepath.Join(tmp, "use-cases", "blue-green")
	applyBody := []byte(`{"output_dir":"` + outDir + `","create_workflow":true,"create_runbook":true}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/use-case-templates/blue-green-release/apply", bytes.NewReader(applyBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("use-case template apply failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, "configs", "apply.yaml")); err != nil {
		t.Fatalf("expected generated use-case config file: %v", err)
	}

	type applyResp struct {
		Workflow struct {
			ID string `json:"id"`
		} `json:"workflow"`
	}
	var created applyResp
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode use-case apply response failed: %v body=%s", err, rr.Body.String())
	}
	if created.Workflow.ID == "" {
		t.Fatalf("expected generated workflow id in use-case apply response")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader([]byte(`{"entity":"use_case_templates","mode":"human","query":"id=blue-green-release","limit":10}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("use-case templates query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/use-case-templates/blue-green-release/apply", bytes.NewReader([]byte(`{"output_dir":"`+outDir+`"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected conflict when output directory exists without overwrite: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestWorkloadCentricViewsEndpoint(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-workload-views.txt")+`
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

	for _, payload := range []string{
		`{"type":"external.alert","message":"latency high","fields":{"workload":"payments","service":"payments-api","host":"pay-01"}}`,
		`{"type":"external.alert","message":"error rate","fields":{"workload":"payments","service":"payments-api","host":"pay-02"}}`,
		`{"type":"external.alert","message":"search warning","fields":{"application":"search","host":"search-01"}}`,
	} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader([]byte(payload)))
		s.httpServer.Handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusAccepted {
			t.Fatalf("event ingest failed: code=%d body=%s", rr.Code, rr.Body.String())
		}
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/views/workloads?limit=10", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("workload views endpoint failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	type workloadResp struct {
		Items []struct {
			Workload   string `json:"workload"`
			AlertCount int    `json:"alert_count"`
			RiskScore  int    `json:"risk_score"`
		} `json:"items"`
	}
	var payload workloadResp
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode workload views response failed: %v body=%s", err, rr.Body.String())
	}
	if len(payload.Items) < 2 {
		t.Fatalf("expected at least two workload entries, got %d", len(payload.Items))
	}
	if payload.Items[0].Workload != "payments" {
		t.Fatalf("expected highest-risk workload to be payments, got %s", payload.Items[0].Workload)
	}
	if payload.Items[0].AlertCount != 2 {
		t.Fatalf("expected payments alert count 2, got %d", payload.Items[0].AlertCount)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader([]byte(`{"entity":"workload_views","mode":"human","query":"workload=payments","limit":10}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("workload views query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestUniversalSearchEndpoint(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-search.txt")+`
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
	req := httptest.NewRequest(http.MethodPost, "/v1/templates", bytes.NewReader([]byte(`{"name":"payments policy","config_path":"c.yaml"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("template create for search failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/runbooks", bytes.NewReader([]byte(`{"name":"payments rollback","target_type":"config","config_path":"c.yaml","risk_level":"high","owner":"sre","tags":["payments"]}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("runbook create for search failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader([]byte(`{"type":"external.alert","message":"payments latency high","fields":{"workload":"payments","service":"payments-api","host":"pay-01"}}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("event ingest for search failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	st := state.New(tmp)
	if err := st.SaveRun(state.RunRecord{
		ID:        "run-search-1",
		StartedAt: time.Now().UTC().Add(-2 * time.Minute),
		EndedAt:   time.Now().UTC(),
		Status:    state.RunSucceeded,
		Results: []state.ResourceRun{
			{ResourceID: "deploy", Type: "file", Host: "pay-01", Changed: true, Message: "updated"},
		},
	}); err != nil {
		t.Fatalf("save run for search failed: %v", err)
	}

	type searchResp struct {
		Items []struct {
			Type  string `json:"type"`
			Title string `json:"title"`
		} `json:"items"`
	}
	hasType := func(items []struct {
		Type  string `json:"type"`
		Title string `json:"title"`
	}, expected string) bool {
		for _, item := range items {
			if item.Type == expected {
				return true
			}
		}
		return false
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/search?q=payments&limit=25", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("search failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var result searchResp
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode search response failed: %v body=%s", err, rr.Body.String())
	}
	if !hasType(result.Items, "service") || !hasType(result.Items, "policy") {
		t.Fatalf("expected service and policy search results, got %#v", result.Items)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/search?q=pay-01&type=host", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("search host filter failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	result = searchResp{}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode host-filter search response failed: %v body=%s", err, rr.Body.String())
	}
	if len(result.Items) == 0 || !hasType(result.Items, "host") {
		t.Fatalf("expected host search results, got %#v", result.Items)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/search?q=blue-green&type=module", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("search module filter failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	result = searchResp{}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode module-filter search response failed: %v body=%s", err, rr.Body.String())
	}
	if len(result.Items) == 0 || !hasType(result.Items, "module") {
		t.Fatalf("expected module search results, got %#v", result.Items)
	}
}

func TestPersonaHomeViews(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-home.txt")+`
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
	req := httptest.NewRequest(http.MethodPost, "/v1/runbooks", bytes.NewReader([]byte(`{"name":"owner-runbook","target_type":"config","config_path":"c.yaml","owner":"payments-team"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("seed runbook for persona home failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	for _, persona := range []string{"sre", "platform", "release", "service-owner"} {
		path := "/v1/views/home?persona=" + persona
		if persona == "service-owner" {
			path += "&owner=payments-team"
		}
		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, path, nil)
		s.httpServer.Handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("persona home failed for %s: code=%d body=%s", persona, rr.Code, rr.Body.String())
		}
		type resp struct {
			Persona string `json:"persona"`
			Cards   []struct {
				ID string `json:"id"`
			} `json:"cards"`
		}
		var payload resp
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode persona home response failed for %s: %v body=%s", persona, err, rr.Body.String())
		}
		if payload.Persona != persona {
			t.Fatalf("expected persona %s, got %s", persona, payload.Persona)
		}
		if len(payload.Cards) == 0 {
			t.Fatalf("expected cards in persona home for %s", persona)
		}
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/views/home?persona=unknown", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for invalid persona: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestIncidentViewEndpoint(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-incident.txt")+`
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
		ID:        "run-incident-1",
		StartedAt: time.Now().UTC().Add(-10 * time.Minute),
		EndedAt:   time.Now().UTC().Add(-8 * time.Minute),
		Status:    state.RunFailed,
		Results: []state.ResourceRun{
			{ResourceID: "deploy", Type: "file", Host: "payments-01", Changed: true, Message: "failed rollout"},
		},
	}); err != nil {
		t.Fatalf("save run for incident view failed: %v", err)
	}

	eventPayload := `{"type":"external.alert","message":"payments saturation","fields":{"workload":"payments","service":"payments-api","run_id":"run-incident-1","dashboard_url":"https://grafana.example.com/d/payments","trace_id":"abc123"}}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader([]byte(eventPayload)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("event ingest for incident view failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/incidents/view?workload=payments&hours=24", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("incident view failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		RiskScore int `json:"risk_score"`
		Events    []struct {
			Type string `json:"type"`
		} `json:"events"`
		Alerts []struct {
			ID string `json:"id"`
		} `json:"alerts"`
		Runs []struct {
			ID string `json:"id"`
		} `json:"runs"`
		ObservabilityLinks []struct {
			URL string `json:"url"`
		} `json:"observability_links"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode incident view failed: %v body=%s", err, rr.Body.String())
	}
	if resp.RiskScore == 0 {
		t.Fatalf("expected non-zero incident risk score")
	}
	if len(resp.Events) == 0 || len(resp.Alerts) == 0 || len(resp.Runs) == 0 {
		t.Fatalf("expected correlated events/alerts/runs in incident response: %+v", resp)
	}
	if len(resp.ObservabilityLinks) == 0 {
		t.Fatalf("expected observability links in incident response")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/incidents/view?run_id=run-incident-1&hours=24", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("incident view by run id failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestBulkPreviewAndExecute(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-bulk.txt")+`
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
	req := httptest.NewRequest(http.MethodPost, "/v1/schedules", bytes.NewReader([]byte(`{"config_path":"c.yaml","interval_seconds":60}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("schedule create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var schedule struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &schedule); err != nil {
		t.Fatalf("schedule decode failed: %v", err)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/runbooks", bytes.NewReader([]byte(`{"name":"bulk-runbook","target_type":"config","config_path":"c.yaml"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("runbook create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var runbook struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &runbook); err != nil {
		t.Fatalf("runbook decode failed: %v", err)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/views", bytes.NewReader([]byte(`{"name":"bulk-view","entity":"alerts","mode":"human","query":"status=open","limit":10}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("view create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var view struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &view); err != nil {
		t.Fatalf("view decode failed: %v", err)
	}

	previewBody := `{
		"name":"bulk-ops",
		"operations":[
			{"action":"schedule.disable","target_type":"schedule","target_id":"` + schedule.ID + `"},
			{"action":"runbook.approve","target_type":"runbook","target_id":"` + runbook.ID + `"},
			{"action":"view.pin","target_type":"view","target_id":"` + view.ID + `"}
		]
	}`
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/bulk/preview", bytes.NewReader([]byte(previewBody)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("bulk preview failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var preview struct {
		Token string `json:"token"`
		Ready bool   `json:"ready"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &preview); err != nil {
		t.Fatalf("bulk preview decode failed: %v", err)
	}
	if preview.Token == "" || !preview.Ready {
		t.Fatalf("expected ready bulk preview token, got %+v", preview)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/bulk/execute", bytes.NewReader([]byte(`{"preview_token":"`+preview.Token+`","confirm":true}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("bulk execute failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/schedules", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"enabled":false`) {
		t.Fatalf("expected schedule to be disabled by bulk execution: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/runbooks/"+runbook.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"status":"approved"`) {
		t.Fatalf("expected runbook approved by bulk execution: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/views/"+view.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"pinned":true`) {
		t.Fatalf("expected view pinned by bulk execution: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestActionDocsEndpoints(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-docs.txt")+`
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
	req := httptest.NewRequest(http.MethodGet, "/v1/docs/actions", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("docs list failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `investigate-failed-run`) {
		t.Fatalf("expected investigate-failed-run doc in list: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/docs/actions/investigate-failed-run", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("doc by id failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader([]byte(`{"entity":"action_docs","mode":"human","query":"id=incident-correlation","limit":10}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("action docs query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/docs/inline?endpoint=POST+/v1/runs/run-123/retry", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("inline docs endpoint lookup failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"investigate-failed-run"`) || !strings.Contains(rr.Body.String(), `"matched_endpoint":"POST /v1/runs/{id}/retry"`) {
		t.Fatalf("expected endpoint-matched inline docs response: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/docs/inline?q=incident&limit=1", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("inline docs query lookup failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"inline_examples":true`) || !strings.Contains(rr.Body.String(), `"count":1`) {
		t.Fatalf("expected inline docs query response metadata: %s", rr.Body.String())
	}
}

func TestWorkflowWizardEndpoints(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-wizards.txt")+`
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
	req := httptest.NewRequest(http.MethodGet, "/v1/wizards", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("wizard list failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"incident-remediation"`) {
		t.Fatalf("expected incident-remediation wizard in list: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/wizards/rollback", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("wizard get failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"execute-rollback"`) {
		t.Fatalf("expected rollback step in wizard definition: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/wizards/rollback/launch", bytes.NewReader([]byte(`{"inputs":{"run_id":"run-1"},"dry_run":true}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("wizard launch validation expected accepted: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"ready":false`) || !strings.Contains(rr.Body.String(), `"rollback_config_path"`) {
		t.Fatalf("expected missing rollback input in launch result: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/wizards/rollout/launch", bytes.NewReader([]byte(`{"inputs":{"config_path":"c.yaml","strategy":"canary","target_environment":"prod"}}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("wizard launch ready path failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"ready":true`) {
		t.Fatalf("expected ready launch response: %s", rr.Body.String())
	}
}

func TestFleetNodesEndpointPaginationAndRenderModes(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-fleet.txt")+`
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
		ID:        "run-fleet-1",
		StartedAt: time.Now().UTC().Add(-5 * time.Minute),
		EndedAt:   time.Now().UTC().Add(-4 * time.Minute),
		Status:    state.RunFailed,
		Results: []state.ResourceRun{
			{ResourceID: "deploy", Type: "file", Host: "node-a", Changed: true},
			{ResourceID: "deploy", Type: "file", Host: "node-b", Changed: true},
		},
	}); err != nil {
		t.Fatalf("save run for fleet endpoint failed: %v", err)
	}

	for _, eventBody := range []string{
		`{"type":"external.alert","message":"node-a alert","fields":{"host":"node-a","workload":"payments"}}`,
		`{"type":"external.alert","message":"node-b alert","fields":{"host":"node-b","workload":"search"}}`,
	} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader([]byte(eventBody)))
		s.httpServer.Handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusAccepted {
			t.Fatalf("event ingest for fleet endpoint failed: code=%d body=%s", rr.Code, rr.Body.String())
		}
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/fleet/nodes?limit=1", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("fleet nodes first page failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var page1 struct {
		Count      int    `json:"count"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &page1); err != nil {
		t.Fatalf("decode fleet nodes page1 failed: %v", err)
	}
	if page1.Count != 1 || page1.NextCursor == "" {
		t.Fatalf("expected paginated fleet response with next cursor, got %+v", page1)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/fleet/nodes?limit=1&cursor="+page1.NextCursor+"&compact=true", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("fleet nodes second page compact failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"workloads"`) {
		t.Fatalf("expected compact fleet response to omit workloads field: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"mode":"compact"`) {
		t.Fatalf("expected compact response mode: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/fleet/nodes?mode=virtualized&limit=1", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("fleet nodes virtualized mode failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"mode":"virtualized"`) ||
		!strings.Contains(rr.Body.String(), `"virtualization"`) ||
		!strings.Contains(rr.Body.String(), `"row_id"`) {
		t.Fatalf("expected virtualized mode payload fields: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/fleet/nodes?mode=low-bandwidth&limit=1", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("fleet nodes low-bandwidth mode failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"mode":"low-bandwidth"`) ||
		!strings.Contains(rr.Body.String(), `"transport_hints"`) ||
		!strings.Contains(rr.Body.String(), `"last_seen_unix"`) {
		t.Fatalf("expected low-bandwidth mode payload fields: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader([]byte(`{"entity":"fleet_nodes","mode":"human","query":"host=node-a","limit":10}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("fleet nodes query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAccessibilityEndpoints(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-accessibility.txt")+`
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
	req := httptest.NewRequest(http.MethodGet, "/v1/ui/accessibility/profiles", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("accessibility profiles list failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"high-contrast"`) {
		t.Fatalf("expected high-contrast built-in profile: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/ui/accessibility/profiles", bytes.NewReader([]byte(`{
		"id":"screenreader-heavy",
		"name":"Screenreader Heavy",
		"keyboard_only":true,
		"screen_reader_optimized":true,
		"high_contrast":true,
		"reduced_motion":true
	}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("accessibility profile upsert failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/ui/accessibility/active", bytes.NewReader([]byte(`{"profile_id":"screenreader-heavy"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set active profile failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"screen_reader_optimized":true`) {
		t.Fatalf("expected optimized active profile: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/ui/accessibility/active", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get active profile failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"id":"screenreader-heavy"`) {
		t.Fatalf("expected custom active profile id in response: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/ui/shortcuts?q=incident&global_only=true", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("shortcut catalog query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"incident-view"`) || !strings.Contains(rr.Body.String(), `"active_profile"`) {
		t.Fatalf("expected incident shortcut and active profile in response: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/ui/progressive-disclosure", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("progressive disclosure list failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"simple"`) || !strings.Contains(rr.Body.String(), `"profiles"`) {
		t.Fatalf("expected progressive disclosure profiles response: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/ui/progressive-disclosure", bytes.NewReader([]byte(`{"profile_id":"balanced","workflow_hint":"rollout"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("progressive disclosure set profile failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"profile_id":"balanced"`) {
		t.Fatalf("expected balanced progressive disclosure state: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/ui/progressive-disclosure/reveal", bytes.NewReader([]byte(`{"workflow":"rollout","controls":["blast radius","failure thresholds"]}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("progressive disclosure reveal failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"blast-radius"`) || !strings.Contains(rr.Body.String(), `"revealed_by_flow"`) {
		t.Fatalf("expected revealed controls in response: %s", rr.Body.String())
	}
}

func TestPlanExplainEndpoint(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	features := filepath.Join(tmp, "features.md")

	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: prep
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "prep.txt")+`
    content: "prep\n"
  - id: deploy
    type: command
    host: localhost
    command: "echo deploy"
    depends_on:
      - prep
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
	req := httptest.NewRequest(http.MethodPost, "/v1/plans/explain", bytes.NewReader([]byte(`{"config_path":"c.yaml"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("plan explain failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	type explainResp struct {
		Steps []struct {
			ResourceID   string   `json:"resource_id"`
			Reason       string   `json:"reason"`
			RiskHint     string   `json:"risk_hint"`
			Dependencies []string `json:"dependencies"`
		} `json:"steps"`
	}
	var resp explainResp
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode plan explain response failed: %v body=%s", err, rr.Body.String())
	}
	if len(resp.Steps) != 2 {
		t.Fatalf("expected 2 explain steps, got %d", len(resp.Steps))
	}
	foundDeploy := false
	for _, step := range resp.Steps {
		if step.ResourceID == "deploy" {
			foundDeploy = true
			if len(step.Dependencies) != 1 || step.Dependencies[0] != "prep" {
				t.Fatalf("expected deploy dependency explanation, got %+v", step.Dependencies)
			}
			if !strings.Contains(step.Reason, "dependencies") {
				t.Fatalf("expected dependency reason in explain output, got %s", step.Reason)
			}
		}
	}
	if !foundDeploy {
		t.Fatalf("expected deploy step in explain output")
	}
}

func TestBlastRadiusMapEndpoint(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	features := filepath.Join(tmp, "features.md")

	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: prep
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "prep-blast.txt")+`
    content: "prep\n"
  - id: deploy
    type: command
    host: localhost
    command: "echo deploy"
    depends_on:
      - prep
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

	body := []byte(`{"config_path":"c.yaml","owners":{"localhost":"platform-team","deploy":"payments-team"}}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/blast-radius-map", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("blast radius map failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"depends_on"`) {
		t.Fatalf("expected dependency edges in blast radius map: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"estimated_scope"`) {
		t.Fatalf("expected blast radius analysis in response: %s", rr.Body.String())
	}
}

func TestPolicySimulationAndRiskSummaryEndpoints(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	features := filepath.Join(tmp, "features.md")

	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: setup
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "setup.txt")+`
    content: "setup\n"
  - id: deploy
    type: command
    host: localhost
    command: "echo deploy"
    depends_on:
      - setup
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
	req := httptest.NewRequest(http.MethodPost, "/v1/policy/simulate", bytes.NewReader([]byte(`{
		"config_path":"c.yaml",
		"deny_resource_types":["command"],
		"minimum_confidence":1.0
	}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("policy simulate failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"would_block_apply":true`) {
		t.Fatalf("expected policy simulation to block apply: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `deploy`) {
		t.Fatalf("expected deploy step in policy simulation: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/plans/risk-summary", bytes.NewReader([]byte(`{"config_path":"c.yaml"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("plan risk summary failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"risk_score"`) || !strings.Contains(rr.Body.String(), `"mitigations"`) {
		t.Fatalf("expected risk summary fields in response: %s", rr.Body.String())
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

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/runs/run-export-1/timeline?minutes_before=10&minutes_after=10", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("run timeline failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var timelineResp struct {
		Count          int            `json:"count"`
		PhaseBreakdown map[string]int `json:"phase_breakdown"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &timelineResp); err != nil {
		t.Fatalf("run timeline decode failed: %v", err)
	}
	if timelineResp.Count == 0 {
		t.Fatalf("expected timeline items for run")
	}
	if timelineResp.PhaseBreakdown["during"] == 0 {
		t.Fatalf("expected during phase timeline items")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/runs/run-export-1/correlations", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("run correlations failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"correlation_id"`) || !strings.Contains(rr.Body.String(), `"trace_url"`) {
		t.Fatalf("expected correlation ids and trace links in response: %s", rr.Body.String())
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

func TestRunRetryAndRollbackActions(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-retry.txt")+`
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
		ID:        "run-recover-1",
		StartedAt: time.Now().UTC().Add(-2 * time.Minute),
		EndedAt:   time.Now().UTC().Add(-time.Minute),
		Status:    state.RunFailed,
		Results: []state.ResourceRun{
			{ResourceID: "f1", Host: "localhost", Type: "file", Changed: true},
		},
	}); err != nil {
		t.Fatalf("save run failed: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/run-recover-1/retry", bytes.NewReader([]byte(`{"config_path":"c.yaml","priority":"high"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("run retry failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var retryResp struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &retryResp); err != nil {
		t.Fatalf("retry response decode failed: %v", err)
	}
	if retryResp.Job.ID == "" {
		t.Fatalf("expected retry job id")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/runs/run-recover-1/rollback", bytes.NewReader([]byte(`{"rollback_config_path":"c.yaml","priority":"high","reason":"deploy regression"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("run rollback failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/runs/run-recover-1/retry", bytes.NewReader([]byte(`{"priority":"high"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected retry validation error for missing config_path: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRunCompareEndpoint(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-compare.txt")+`
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
		ID:        "run-compare-failed",
		StartedAt: time.Now().UTC().Add(-3 * time.Minute),
		EndedAt:   time.Now().UTC().Add(-2 * time.Minute),
		Status:    state.RunFailed,
		Results: []state.ResourceRun{
			{ResourceID: "f1", Type: "file", Host: "localhost", Changed: true, Message: "write failed"},
		},
	}); err != nil {
		t.Fatalf("save failed run: %v", err)
	}
	if err := st.SaveRun(state.RunRecord{
		ID:        "run-compare-success",
		StartedAt: time.Now().UTC().Add(-90 * time.Second),
		EndedAt:   time.Now().UTC().Add(-30 * time.Second),
		Status:    state.RunSucceeded,
		Results: []state.ResourceRun{
			{ResourceID: "f1", Type: "file", Host: "localhost", Changed: false, Message: "already converged"},
		},
	}); err != nil {
		t.Fatalf("save success run: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/compare?run_a=run-compare-failed&run_b=run-compare-success", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("run compare failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"diff_count"`) {
		t.Fatalf("expected diff_count in run compare response: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `changed flag differs`) {
		t.Fatalf("expected changed flag diff in response: %s", rr.Body.String())
	}
}

func TestDriftInsightsEndpoint(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-drift.txt")+`
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
	for i := 0; i < 3; i++ {
		if err := st.SaveRun(state.RunRecord{
			ID:        "run-drift-" + strconv.Itoa(i),
			StartedAt: time.Now().UTC().Add(-time.Duration(10-i) * time.Minute),
			EndedAt:   time.Now().UTC().Add(-time.Duration(9-i) * time.Minute),
			Status:    state.RunFailed,
			Results: []state.ResourceRun{
				{ResourceID: "cmd", Type: "command", Host: "node-a", Changed: true, Message: "changed"},
			},
		}); err != nil {
			t.Fatalf("save drift run %d failed: %v", i, err)
		}
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/drift/insights?hours=24", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("drift insights failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"root_cause_hints"`) || !strings.Contains(rr.Body.String(), `"remediations"`) {
		t.Fatalf("expected root-cause hints and remediations in drift insights: %s", rr.Body.String())
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

func TestInvariantChecksEndpoint(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-invariants.txt")+`
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

	passBody := []byte(`{
		"invariants":[
			{"name":"error-rate","field":"error_rate","comparator":"lte","value":0.02,"severity":"critical"},
			{"name":"latency","field":"p95_ms","comparator":"lte","value":600,"severity":"warning"}
		],
		"observed":{"error_rate":0.01,"p95_ms":700}
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/invariants/check", bytes.NewReader(passBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected invariant check pass: code=%d body=%s", rr.Code, rr.Body.String())
	}

	failBody := []byte(`{
		"invariants":[
			{"name":"error-rate","field":"error_rate","comparator":"lte","value":0.02,"severity":"critical"}
		],
		"observed":{"error_rate":0.2}
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/invariants/check", bytes.NewReader(failBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected invariant check conflict on critical failure: code=%d body=%s", rr.Code, rr.Body.String())
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

func TestReleaseBlockerPolicyEndpoint(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-release-blocker.txt")+`
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
	req := httptest.NewRequest(http.MethodGet, "/v1/release/blocker-policy", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("release blocker policy get failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	failBody := []byte(`{
		"signals":{"quality_score":0.7,"reliability_score":0.7,"performance_score":0.7,"test_pass_rate":0.7,"flake_rate":0.1,"open_critical_incidents":1,"p95_apply_latency_ms":999999},
		"simulation_confidence":0.5
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/blocker-policy", bytes.NewReader(failBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected release blocker conflict: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"craftsmanship_tier":"bronze"`) {
		t.Fatalf("expected bronze tier when blocked: %s", rr.Body.String())
	}

	passBody := []byte(`{
		"signals":{"quality_score":0.97,"reliability_score":0.97,"performance_score":0.96,"test_pass_rate":0.995,"flake_rate":0.005,"open_critical_incidents":0,"p95_apply_latency_ms":1200},
		"simulation_confidence":0.99
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/blocker-policy", bytes.NewReader(passBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected release blocker pass: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"craftsmanship_tier":"gold"`) {
		t.Fatalf("expected gold tier when passing: %s", rr.Body.String())
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
