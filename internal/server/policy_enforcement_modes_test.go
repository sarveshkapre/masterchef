package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicyEnforcementModeEndpoints(t *testing.T) {
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

	upsert := []byte(`{"policy_ref":"policy-prod","mode":"apply-and-autocorrect","reason":"prod drift policy","updated_by":"sre"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/policy/enforcement-modes", bytes.NewReader(upsert))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upsert enforcement mode failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"mode":"apply-and-autocorrect"`) {
		t.Fatalf("expected upserted mode in response: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/policy/enforcement-modes/policy-prod", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get enforcement mode failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/policy/enforcement-modes", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list enforcement modes failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"policy_ref":"policy-prod"`) {
		t.Fatalf("expected policy in list response: %s", rr.Body.String())
	}

	evaluate := []byte(`{"policy_ref":"policy-prod","drift_detected":true,"high_risk":false,"can_autocorrect":true,"simulation_confidence_percent":95}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/policy/enforcement-modes/evaluate", bytes.NewReader(evaluate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("evaluate enforcement mode failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"action":"apply-and-autocorrect"`) {
		t.Fatalf("expected autocorrect action in evaluation response: %s", rr.Body.String())
	}

	highRisk := []byte(`{"policy_ref":"policy-prod","drift_detected":true,"high_risk":true,"can_autocorrect":true,"simulation_confidence_percent":50}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/policy/enforcement-modes/evaluate", bytes.NewReader(highRisk))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("high-risk evaluation failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"decision":"blocked"`) || !strings.Contains(rr.Body.String(), `"action":"monitor-only"`) {
		t.Fatalf("expected blocked monitor-only action for high risk: %s", rr.Body.String())
	}
}
