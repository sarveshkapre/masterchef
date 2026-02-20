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

func TestGitOpsPRReviewEndpoints(t *testing.T) {
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

	gateBody := []byte(`{
		"repository":"github.com/masterchef/masterchef",
		"environment":"prod",
		"min_approvals":2,
		"required_checks":["plan/simulate","plan/reproducibility"],
		"required_reviewers":["platform-oncall"],
		"block_risk_levels":["high","critical"]
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/gitops/approval-gates", bytes.NewReader(gateBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upsert approval gate failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var gate struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &gate); err != nil {
		t.Fatalf("decode gate response failed: %v", err)
	}
	if gate.ID == "" {
		t.Fatalf("expected approval gate id")
	}

	commentBody := []byte(`{
		"repository":"github.com/masterchef/masterchef",
		"pr_number":42,
		"environment":"prod",
		"plan_summary":"Touches 12 hosts and 3 services",
		"risk_level":"high",
		"suggested_actions":["require 3 approvals","verify canary health"]
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/gitops/pr-comments", bytes.NewReader(commentBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("post pr comment failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/gitops/pr-comments?repository=github.com/masterchef/masterchef&pr_number=42", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list pr comments failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	blockedEval := []byte(`{
		"repository":"github.com/masterchef/masterchef",
		"environment":"prod",
		"pr_number":42,
		"risk_level":"high",
		"approval_count":2,
		"passed_checks":["plan/simulate"],
		"reviewers":["platform-oncall"]
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/gitops/approval-gates/evaluate", bytes.NewReader(blockedEval))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected gate evaluation conflict for high risk: code=%d body=%s", rr.Code, rr.Body.String())
	}

	allowedEval := []byte(`{
		"gate_id":"` + gate.ID + `",
		"repository":"github.com/masterchef/masterchef",
		"environment":"prod",
		"pr_number":42,
		"risk_level":"medium",
		"approval_count":2,
		"passed_checks":["plan/simulate","plan/reproducibility"],
		"reviewers":["platform-oncall"]
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/gitops/approval-gates/evaluate", bytes.NewReader(allowedEval))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected gate evaluation pass: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
