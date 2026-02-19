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

func TestChaosExperimentEndpoints(t *testing.T) {
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

	asyncCreate := []byte(`{
		"name":"queue-chaos",
		"target":"staging/queue",
		"fault_type":"queue-delay",
		"intensity":50,
		"duration_sec":60,
		"async":true
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/chaos/experiments", bytes.NewReader(asyncCreate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create async chaos experiment failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var exp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &exp); err != nil {
		t.Fatalf("decode async experiment failed: %v", err)
	}
	if exp.ID == "" || exp.Status != "running" {
		t.Fatalf("unexpected async experiment payload: %+v", exp)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/chaos/experiments/"+exp.ID+"/complete", bytes.NewReader([]byte(`{}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("complete async chaos experiment failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	blockedCreate := []byte(`{
		"name":"prod-chaos",
		"target":"prod/payments",
		"fault_type":"network-latency",
		"intensity":90,
		"duration_sec":60
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/chaos/experiments", bytes.NewReader(blockedCreate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create blocked chaos experiment failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/chaos/experiments", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list chaos experiments failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
