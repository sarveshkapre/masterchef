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

func TestRoleEnvironmentEndpointsAndFileBackedReload(t *testing.T) {
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

	roleBody := []byte(`{
		"name":"web",
		"description":"web tier",
		"run_list":["recipe[base]","recipe[web]"],
		"default_attributes":{"region":"us-east-1","level":"role-default"},
		"override_attributes":{"level":"role-override"}
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/roles", bytes.NewReader(roleBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create role failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	envBody := []byte(`{
		"name":"prod",
		"policy_group":"stable",
		"default_attributes":{"tier":"critical","level":"env-default"},
		"override_attributes":{"level":"env-override"},
		"run_list_overrides":{"web":["recipe[base]","recipe[web-hardening]"]},
		"policy_overrides":{"release_window":"on"}
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/environments", bytes.NewReader(envBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create environment failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/roles/web/resolve?environment=prod", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("resolve role/environment failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var resolution struct {
		RunList    []string       `json:"run_list"`
		Attributes map[string]any `json:"attributes"`
		Policy     string         `json:"policy_group"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resolution); err != nil {
		t.Fatalf("decode resolution failed: %v", err)
	}
	if len(resolution.RunList) != 2 || resolution.RunList[1] != "recipe[web-hardening]" {
		t.Fatalf("unexpected resolved run list: %#v", resolution.RunList)
	}
	if resolution.Attributes["level"] != "env-override" || resolution.Attributes["release_window"] != "on" {
		t.Fatalf("unexpected resolved attributes: %#v", resolution.Attributes)
	}
	if resolution.Policy != "stable" {
		t.Fatalf("unexpected resolved policy group: %s", resolution.Policy)
	}

	queryBody := []byte(`{"entity":"roles","mode":"human","query":"name=web","limit":10}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader(queryBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("query roles failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var queryResp struct {
		MatchedCount int `json:"matched_count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &queryResp); err != nil {
		t.Fatalf("decode query response failed: %v", err)
	}
	if queryResp.MatchedCount != 1 {
		t.Fatalf("expected one role match, got %d", queryResp.MatchedCount)
	}

	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	reloaded := New(":0", tmp)
	t.Cleanup(func() {
		_ = reloaded.Shutdown(context.Background())
	})

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/roles/web", nil)
	reloaded.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected role to load from file-backed store: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/v1/roles/web", nil)
	reloaded.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete role failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/v1/environments/prod", nil)
	reloaded.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete environment failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
