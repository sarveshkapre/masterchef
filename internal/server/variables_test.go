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

func TestVariableResolveAndExplainEndpoints(t *testing.T) {
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

	roleBody := []byte(`{
		"name":"web",
		"default_attributes":{"db":{"host":"db-role","port":5432}},
		"override_attributes":{"db":{"host":"db-role-override"}}
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/roles", bytes.NewReader(roleBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create role failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	envBody := []byte(`{
		"name":"prod",
		"override_attributes":{"db":{"host":"db-prod"}},
		"policy_overrides":{"region":"us-east-1"}
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/environments", bytes.NewReader(envBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create env failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	dataBagBody := []byte(`{
		"bag":"shared",
		"item":"app",
		"data":{"db":{"host":"db-databag"}}
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/data-bags", bytes.NewReader(dataBagBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create data bag failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	resolveBody := []byte(`{
		"layers":[
			{"name":"global","data":{"db":{"host":"db-global"}}}
		],
		"include_role":"web",
		"include_environment":"prod",
		"include_data_bags":["shared/app"]
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/vars/resolve", bytes.NewReader(resolveBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("resolve vars failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var resolveResp struct {
		Result struct {
			Merged map[string]any `json:"merged"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resolveResp); err != nil {
		t.Fatalf("decode resolve response failed: %v", err)
	}
	db, ok := resolveResp.Result.Merged["db"].(map[string]any)
	if !ok || db["host"] != "db-databag" {
		t.Fatalf("expected data bag to win final precedence, got %#v", resolveResp.Result.Merged["db"])
	}

	explainBody := []byte(`{
		"hard_fail":true,
		"layers":[
			{"name":"global","data":{"region":"us-east-1"}},
			{"name":"environment","data":{"region":"us-west-2"}}
		]
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/vars/explain", bytes.NewReader(explainBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected hard-fail conflict status, got code=%d body=%s", rr.Code, rr.Body.String())
	}
}
