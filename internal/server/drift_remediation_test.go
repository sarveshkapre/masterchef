package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/masterchef/masterchef/internal/state"
)

func TestDriftRemediationEndpoint_EnqueueAndSafeModeBlock(t *testing.T) {
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

	st := state.New(tmp)
	if err := st.SaveRun(state.RunRecord{
		ID:        "run-ok",
		StartedAt: time.Now().UTC().Add(-5 * time.Minute),
		EndedAt:   time.Now().UTC().Add(-4 * time.Minute),
		Status:    state.RunSucceeded,
		Results: []state.ResourceRun{
			{ResourceID: "r1", Type: "command", Host: "node-a", Changed: true, Message: "changed"},
		},
	}); err != nil {
		t.Fatalf("save run failed: %v", err)
	}

	body := []byte(`{"hours":24,"config_path":"c.yaml","priority":"high","safe_mode":true,"max_changes":10}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/drift/remediate", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected remediation enqueue success: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"enqueued"`) || !strings.Contains(rr.Body.String(), `"job_id":"job-`) {
		t.Fatalf("expected enqueued response with job id: %s", rr.Body.String())
	}

	for i := 0; i < 30; i++ {
		if err := st.SaveRun(state.RunRecord{
			ID:        "run-risk-" + strconv.Itoa(i),
			StartedAt: time.Now().UTC().Add(-3 * time.Minute),
			EndedAt:   time.Now().UTC().Add(-2 * time.Minute),
			Status:    state.RunFailed,
			Results: []state.ResourceRun{
				{ResourceID: "risk-" + strconv.Itoa(i), Type: "command", Host: "node-b", Changed: true, Message: "changed"},
			},
		}); err != nil {
			t.Fatalf("save risk run %d failed: %v", i, err)
		}
	}

	blockBody := []byte(`{"hours":24,"config_path":"c.yaml","safe_mode":true,"max_changes":1}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/drift/remediate", bytes.NewReader(blockBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected safe mode block: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"blocked"`) {
		t.Fatalf("expected blocked status in response: %s", rr.Body.String())
	}
}
