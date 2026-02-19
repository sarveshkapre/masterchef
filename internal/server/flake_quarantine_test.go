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

func TestFlakeQuarantineEndpoints(t *testing.T) {
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

	policy := []byte(`{"auto_quarantine":true,"min_samples":3,"flake_rate_threshold":0.5,"consecutive_failure_threshold":2}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/release/tests/flake-policy", bytes.NewReader(policy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set flake policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	var last struct {
		Case struct {
			ID          string `json:"id"`
			Quarantined bool   `json:"quarantined"`
		} `json:"case"`
		Action string `json:"action"`
	}
	for _, status := range []string{"fail", "pass", "fail"} {
		body := []byte(`{"suite":"e2e","test":"TestRollout","status":"` + status + `"}`)
		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/v1/release/tests/flake-observations", bytes.NewReader(body))
		s.httpServer.Handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("flake observation failed: code=%d body=%s", rr.Code, rr.Body.String())
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &last); err != nil {
			t.Fatalf("decode flake observation failed: %v", err)
		}
	}
	if last.Case.ID == "" || !last.Case.Quarantined || last.Action != "auto-quarantined" {
		t.Fatalf("expected auto quarantined case, got %+v", last)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/release/tests/flake-cases?filter=quarantined", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list quarantined flake cases failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/tests/flake-cases/"+last.Case.ID+"/unquarantine", bytes.NewReader([]byte(`{"reason":"stabilized"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unquarantine flake case failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/tests/flake-cases/"+last.Case.ID+"/quarantine", bytes.NewReader([]byte(`{"reason":"manual block"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("manual quarantine flake case failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/release/tests/flake-cases/"+last.Case.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get flake case failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
