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

func TestNodeClassificationEndpoints(t *testing.T) {
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

	ruleBody := []byte(`{"name":"prod-linux","match_labels":{"env":"prod"},"match_facts":{"os":"linux"},"policy_group":"production","run_list":["role[base]","role[web]"],"priority":100,"enabled":true}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/inventory/classification-rules", bytes.NewReader(ruleBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create classification rule failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var rule struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &rule); err != nil {
		t.Fatalf("decode created rule: %v", err)
	}
	if rule.ID == "" {
		t.Fatalf("expected rule id")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/inventory/classification-rules/"+rule.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get classification rule failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	classify := []byte(`{"node":"web-1","facts":{"os":"linux"},"labels":{"env":"prod"}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/inventory/classify", bytes.NewReader(classify))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("classify node failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"policy_group":"production"`) {
		t.Fatalf("expected policy_group=production, body=%s", rr.Body.String())
	}
}
