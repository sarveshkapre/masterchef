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

func TestHealthProbeEndpoints(t *testing.T) {
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
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })

	createProbe := []byte(`{"name":"payments-api","service":"payments","endpoint":"https://payments/healthz","enabled":true}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/health-probes", bytes.NewReader(createProbe))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create probe target failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"id":"probe-`) {
		t.Fatalf("expected probe target id in response: %s", rr.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode probe create response failed: %v", err)
	}
	probeID := created.ID
	if probeID == "" {
		t.Fatalf("expected probe id")
	}

	checkHealthy := []byte(`{"target_id":"` + probeID + `","status":"healthy","latency_ms":12}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/health-probes/checks", bytes.NewReader(checkHealthy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("record healthy check failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	evaluateAllow := []byte(`{"target_ids":["` + probeID + `"],"min_healthy_percent":100}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/health-probes/evaluate", bytes.NewReader(evaluateAllow))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("evaluate allow failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"decision":"allow"`) {
		t.Fatalf("expected allow decision: %s", rr.Body.String())
	}

	checkUnhealthy := []byte(`{"target_id":"` + probeID + `","status":"unhealthy","latency_ms":300}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/health-probes/checks", bytes.NewReader(checkUnhealthy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("record unhealthy check failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	evaluateBlock := []byte(`{"target_ids":["` + probeID + `"],"min_healthy_percent":100,"recommend_rollback":true}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/health-probes/evaluate", bytes.NewReader(evaluateBlock))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("evaluate block failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"decision":"block"`) || !strings.Contains(rr.Body.String(), `"recommended_action":"rollback"`) {
		t.Fatalf("expected blocked rollback recommendation: %s", rr.Body.String())
	}
}
