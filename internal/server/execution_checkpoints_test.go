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

func TestExecutionCheckpointResumeEndpoint(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "resume.yaml")
	features := filepath.Join(tmp, "features.md")
	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: a
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "a.txt")+`
    content: "a"
  - id: b
    type: file
    host: localhost
    depends_on: [a]
    path: `+filepath.Join(tmp, "b.txt")+`
    content: "b"
  - id: c
    type: command
    host: localhost
    depends_on: [b]
    command: "echo c"
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

	record := []byte(`{"config_path":"resume.yaml","step_id":"a","step_order":1,"status":"succeeded"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/execution/checkpoints", bytes.NewReader(record))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("record checkpoint failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var checkpoint struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &checkpoint); err != nil {
		t.Fatalf("decode checkpoint: %v", err)
	}

	resume := []byte(`{"checkpoint_id":"` + checkpoint.ID + `","priority":"high"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/checkpoints/resume", bytes.NewReader(resume))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("resume checkpoint failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"remaining_steps":2`) {
		t.Fatalf("expected remaining_steps=2: %s", rr.Body.String())
	}
	var resumeResp struct {
		ResumeConfigPath string `json:"resume_config_path"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resumeResp); err != nil {
		t.Fatalf("decode resume response: %v", err)
	}
	content, err := os.ReadFile(resumeResp.ResumeConfigPath)
	if err != nil {
		t.Fatalf("read resume config: %v", err)
	}
	if strings.Contains(string(content), `"id": "a"`) {
		t.Fatalf("resume config should not include completed step a: %s", string(content))
	}
	if !strings.Contains(string(content), `"id": "b"`) || !strings.Contains(string(content), `"id": "c"`) {
		t.Fatalf("resume config should include remaining steps b and c: %s", string(content))
	}
}
