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

func TestQueueBacklogSLOPolicyAndStatusEndpoints(t *testing.T) {
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
    path: `+filepath.Join(tmp, "x-backlog-slo.txt")+`
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

	policyBody := []byte(`{"threshold":1,"warning_percent":60,"recovery_percent":40,"projection_seconds":300}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/queue/backlog-slo/policy", bytes.NewReader(policyBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set queue backlog slo policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/queue", bytes.NewReader([]byte(`{"action":"pause"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("pause queue failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewReader([]byte(`{"config_path":"c.yaml","priority":"high"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("enqueue job failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/queue/backlog-slo/status?limit=5", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get queue backlog slo status failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var statusResp struct {
		Policy struct {
			Threshold int `json:"threshold"`
		} `json:"policy"`
		Saturation bool `json:"saturation"`
		Latest     struct {
			State     string `json:"state"`
			Threshold int    `json:"threshold"`
		} `json:"latest"`
		History []struct {
			State string `json:"state"`
		} `json:"history"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("decode queue backlog slo status failed: %v body=%s", err, rr.Body.String())
	}
	if statusResp.Policy.Threshold != 1 {
		t.Fatalf("expected policy threshold=1, got %+v", statusResp.Policy)
	}
	if statusResp.Latest.State == "" {
		t.Fatalf("expected latest backlog state to be populated, got %+v", statusResp)
	}
	if statusResp.Latest.Threshold != 1 || len(statusResp.History) == 0 {
		t.Fatalf("expected latest threshold and status history, got %+v", statusResp)
	}
	_ = statusResp.Saturation
}
