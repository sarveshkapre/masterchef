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

func TestTaskFrameworkEndpoints(t *testing.T) {
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
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	taskReq := []byte(`{
  "name":"Deploy Service",
  "module":"packs/web",
  "action":"deploy",
  "primitive":"module_action",
  "parameters":[
    {"name":"service","type":"string","required":true},
    {"name":"token","type":"string","required":true,"sensitive":true},
    {"name":"replicas","type":"integer","default":2}
  ]
}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/definitions", bytes.NewReader(taskReq))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("task create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var task struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &task); err != nil {
		t.Fatalf("decode task create failed: %v", err)
	}
	if task.ID == "" {
		t.Fatalf("expected task id")
	}

	planReq := []byte(`{
  "name":"prod rollout",
  "steps":[{
    "name":"deploy-step",
    "task_id":"` + task.ID + `",
    "parameters":{
      "service":"api",
      "token":"secret-token"
    }
  }]
}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/tasks/plans", bytes.NewReader(planReq))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("plan create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "secret-token") {
		t.Fatalf("expected sensitive parameters to be masked in plan response: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "***REDACTED***") {
		t.Fatalf("expected masked sentinel in plan response: %s", rr.Body.String())
	}
	var plan struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &plan); err != nil {
		t.Fatalf("decode plan create failed: %v", err)
	}

	previewReq := []byte(`{"overrides":{"deploy-step":{"token":"override-secret","replicas":3}}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/tasks/plans/"+plan.ID+"/preview", bytes.NewReader(previewReq))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("preview failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "override-secret") {
		t.Fatalf("expected sensitive override to be masked in preview: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "\"sensitive_fields\":[\"token\"]") {
		t.Fatalf("expected sensitive_fields metadata in preview: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/tasks/plans/"+plan.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("plan get failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "secret-token") {
		t.Fatalf("expected masked parameters on get plan: %s", rr.Body.String())
	}
}

func TestTaskFrameworkTypeValidation(t *testing.T) {
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

	taskReq := []byte(`{"name":"Scale","module":"packs/scale","action":"set","parameters":[{"name":"replicas","type":"integer","required":true}]}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/definitions", bytes.NewReader(taskReq))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("task create failed: %d %s", rr.Code, rr.Body.String())
	}
	var task struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &task); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	badPlan := []byte(`{"name":"bad","steps":[{"task_id":"` + task.ID + `","parameters":{"replicas":"three"}}]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/tasks/plans", bytes.NewReader(badPlan))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for invalid type, got %d body=%s", rr.Code, rr.Body.String())
	}
}
