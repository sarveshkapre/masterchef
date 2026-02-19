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

func TestUpgradeOrchestrationEndpoints(t *testing.T) {
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

	planBody := []byte(`{"component":"agent","from_channel":"stable","to_channel":"candidate","strategy":"wave","total_nodes":5,"wave_size":2,"max_unavailable":1}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/upgrade-orchestration/plans", bytes.NewReader(planBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create upgrade plan failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var plan struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &plan)
	if plan.ID == "" {
		t.Fatalf("expected upgrade plan id")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/upgrade-orchestration/plans/"+plan.ID+"/advance", bytes.NewReader([]byte(`{"healthy":true}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("advance upgrade plan failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/upgrade-orchestration/plans/"+plan.ID+"/advance", bytes.NewReader([]byte(`{"healthy":false}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected blocked wave conflict response: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
