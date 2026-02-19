package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/state"
)

func TestDriftHistoryEndpoint(t *testing.T) {
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
	now := time.Now().UTC()
	if err := st.SaveRun(state.RunRecord{
		ID:        "history-1",
		StartedAt: now.Add(-12 * time.Minute),
		EndedAt:   now.Add(-11 * time.Minute),
		Status:    state.RunSucceeded,
		Results: []state.ResourceRun{
			{ResourceID: "f1", Type: "file", Host: "node-a", Changed: true, Message: "updated"},
			{ResourceID: "c1", Type: "command", Host: "node-a", Changed: false, Message: "no-op"},
		},
	}); err != nil {
		t.Fatalf("save run failed: %v", err)
	}
	if err := st.SaveRun(state.RunRecord{
		ID:        "history-2",
		StartedAt: now.Add(-6 * time.Minute),
		EndedAt:   now.Add(-5 * time.Minute),
		Status:    state.RunFailed,
		Results: []state.ResourceRun{
			{ResourceID: "c2", Type: "command", Host: "node-b", Changed: true, Message: "failed"},
		},
	}); err != nil {
		t.Fatalf("save run failed: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/drift/history?hours=24&limit=10", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("drift history failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"count":2`) || !strings.Contains(rr.Body.String(), `"resource_id":"c2"`) {
		t.Fatalf("unexpected drift history response: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"resource_id":"c1"`) {
		t.Fatalf("expected unchanged resource to be excluded by default: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/drift/history?hours=24&host=node-a&type=file&include_unchanged=true&limit=10", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("filtered drift history failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"count":1`) || !strings.Contains(rr.Body.String(), `"resource_id":"f1"`) {
		t.Fatalf("unexpected filtered history response: %s", rr.Body.String())
	}
}

func TestDriftHistorySuppressionAllowlistFilters(t *testing.T) {
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
	now := time.Now().UTC()
	_, _ = s.driftPolicies.AddSuppression(control.DriftSuppressionInput{
		ScopeType:  "host",
		ScopeValue: "node-sup",
		Until:      now.Add(30 * time.Minute),
	})
	_, _ = s.driftPolicies.AddAllowlist(control.DriftAllowlistInput{
		ScopeType:  "resource_id",
		ScopeValue: "r-allow",
	})
	st := state.New(tmp)
	_ = st.SaveRun(state.RunRecord{
		ID:        "history-flags",
		StartedAt: now.Add(-2 * time.Minute),
		EndedAt:   now.Add(-1 * time.Minute),
		Status:    state.RunSucceeded,
		Results: []state.ResourceRun{
			{ResourceID: "r-sup", Type: "file", Host: "node-sup", Changed: true, Message: "suppressed"},
			{ResourceID: "r-allow", Type: "file", Host: "node-a", Changed: true, Message: "allowlisted"},
		},
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/drift/history?hours=24", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("drift history failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"count":0`) {
		t.Fatalf("expected default history to suppress/allowlist filter entries: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/drift/history?hours=24&include_suppressed=true&include_allowlisted=true", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("drift history include flags failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"count":2`) || !strings.Contains(rr.Body.String(), `"suppressed":true`) || !strings.Contains(rr.Body.String(), `"allowlisted":true`) {
		t.Fatalf("expected suppressed/allowlisted items in response: %s", rr.Body.String())
	}
}
