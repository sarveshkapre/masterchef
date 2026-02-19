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

	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/planner"
)

func TestPlanReproducibilityEndpoint(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	features := filepath.Join(tmp, "features.md")
	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: x
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "x.txt")+`
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

	plan := &planner.Plan{Steps: []planner.Step{{Order: 1, Host: config.Host{Name: "localhost", Transport: "local"}, Resource: config.Resource{ID: "x", Type: "file", Host: "localhost", Path: filepath.Join(tmp, "x.txt"), Content: "ok"}}}}
	baseline := filepath.Join(tmp, "baseline.plan.json")
	runnerA := filepath.Join(tmp, "runner-a.plan.json")
	runnerB := filepath.Join(tmp, "runner-b.plan.json")
	if err := planner.SaveSnapshot(baseline, plan); err != nil {
		t.Fatal(err)
	}
	if err := planner.SaveSnapshot(runnerA, plan); err != nil {
		t.Fatal(err)
	}
	if err := planner.SaveSnapshot(runnerB, plan); err != nil {
		t.Fatal(err)
	}

	s := New(":0", tmp)
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })

	body := []byte(`{"baseline_path":"` + baseline + `","runner_plans":[{"runner":"a","plan_path":"` + runnerA + `"},{"runner":"b","plan_path":"` + runnerB + `"}]}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/plans/reproducibility-check", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reproducibility check failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"reproducible":true`)) {
		t.Fatalf("expected reproducible true, got %s", rr.Body.String())
	}
}

func TestPlanReproducibilityEndpointMismatch(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	features := filepath.Join(tmp, "features.md")
	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: x
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "x.txt")+`
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

	baselinePlan := &planner.Plan{Steps: []planner.Step{{Order: 1, Host: config.Host{Name: "localhost", Transport: "local"}, Resource: config.Resource{ID: "x", Type: "file", Host: "localhost", Path: "/tmp/x", Content: "ok"}}}}
	runnerPlan := &planner.Plan{Steps: []planner.Step{{Order: 1, Host: config.Host{Name: "localhost", Transport: "local"}, Resource: config.Resource{ID: "x", Type: "file", Host: "localhost", Path: "/tmp/x", Content: "changed"}}}}
	baseline := filepath.Join(tmp, "baseline.plan.json")
	runner := filepath.Join(tmp, "runner.plan.json")
	if err := planner.SaveSnapshot(baseline, baselinePlan); err != nil {
		t.Fatal(err)
	}
	if err := planner.SaveSnapshot(runner, runnerPlan); err != nil {
		t.Fatal(err)
	}

	s := New(":0", tmp)
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })

	body := []byte(`{"baseline_path":"` + baseline + `","runner_plans":[{"runner":"a","plan_path":"` + runner + `"}]}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/plans/reproducibility-check", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reproducibility check failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Reproducible bool `json:"reproducible"`
		Results      []struct {
			Diff struct {
				ChangedSteps []string `json:"changed_steps"`
			} `json:"diff"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if resp.Reproducible {
		t.Fatalf("expected reproducible false")
	}
	if len(resp.Results) != 1 || len(resp.Results[0].Diff.ChangedSteps) != 1 {
		t.Fatalf("expected changed step in mismatch response, got %+v", resp)
	}
}
