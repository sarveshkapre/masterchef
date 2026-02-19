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

func TestDependencyUpdateBotEndpoints(t *testing.T) {
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

	policy := []byte(`{"enabled":true,"max_updates_per_day":10,"require_compatibility_check":true,"require_performance_check":true,"allowed_ecosystems":["go"]}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/release/dependency-bot/policy", bytes.NewReader(policy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set dependency policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	propose := []byte(`{"ecosystem":"go","package":"github.com/example/pkg","current_version":"v1.0.0","target_version":"v1.1.0","reason":"security fix"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/dependency-bot/updates", bytes.NewReader(propose))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("propose dependency update failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var item struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &item)
	if item.ID == "" {
		t.Fatalf("expected update id")
	}

	evaluate := []byte(`{"compatibility_checked":true,"compatibility_passed":true,"performance_checked":true,"performance_delta_pct":1.0}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/dependency-bot/updates/"+item.ID+"/evaluate", bytes.NewReader(evaluate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("evaluate dependency update failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/release/dependency-bot/updates/"+item.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get dependency update failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
