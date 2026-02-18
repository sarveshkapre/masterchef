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

func TestPillarResolveEndpoint(t *testing.T) {
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
		"override_attributes":{"owner":"platform"}
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/roles", bytes.NewReader(roleBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("role create failed: code=%d body=%s", rr.Code, rr.Body.String())
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
		t.Fatalf("environment create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	bagBody := []byte(`{
		"bag":"shared",
		"item":"app",
		"data":{"db":{"user":"svc"}}
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/data-bags", bytes.NewReader(bagBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("data bag create failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	resolveBody := []byte(`{
		"strategy":"merge-last",
		"role":"web",
		"environment":"prod",
		"data_bag_refs":["shared/app"],
		"lookup":"db.host"
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/pillar/resolve", bytes.NewReader(resolveBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("pillar resolve failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Result struct {
			Found  bool           `json:"found"`
			Value  any            `json:"value"`
			Merged map[string]any `json:"merged"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resolve response failed: %v", err)
	}
	if !resp.Result.Found || resp.Result.Value != "db-prod" {
		t.Fatalf("unexpected lookup value: %+v", resp.Result)
	}
	db, ok := resp.Result.Merged["db"].(map[string]any)
	if !ok {
		t.Fatalf("expected db merged map: %#v", resp.Result.Merged["db"])
	}
	if db["user"] != "svc" || db["port"] != float64(5432) {
		t.Fatalf("expected merged db user+port fields, got %#v", db)
	}
}
