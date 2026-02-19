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

func TestTestImpactAnalysisEndpoint(t *testing.T) {
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

	targetedBody := []byte(`{
		"changed_files":["internal/server/server.go","internal/control/queue.go"],
		"always_include":["./internal/features"],
		"max_targeted_packages":10
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/release/tests/impact-analysis", bytes.NewReader(targetedBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("targeted impact analysis failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var targeted struct {
		Scope           string   `json:"scope"`
		FallbackToAll   bool     `json:"fallback_to_all"`
		ImpactedPackage []string `json:"impacted_packages"`
		RecommendedTest string   `json:"recommended_test"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &targeted); err != nil {
		t.Fatalf("decode targeted analysis: %v", err)
	}
	if targeted.Scope != "targeted" || targeted.FallbackToAll {
		t.Fatalf("expected targeted mode without fallback, got %+v", targeted)
	}
	if !strings.Contains(targeted.RecommendedTest, "go test ") {
		t.Fatalf("expected recommended test command, got %+v", targeted)
	}

	fallbackBody := []byte(`{
		"changed_files":["README.md","internal/storage/store.go","internal/state/state.go"],
		"max_targeted_packages":1
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/release/tests/impact-analysis", bytes.NewReader(fallbackBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("fallback impact analysis failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var fallback struct {
		Scope         string `json:"scope"`
		FallbackToAll bool   `json:"fallback_to_all"`
		Reason        string `json:"reason"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &fallback); err != nil {
		t.Fatalf("decode fallback analysis: %v", err)
	}
	if fallback.Scope != "safe-fallback" || !fallback.FallbackToAll || fallback.Reason == "" {
		t.Fatalf("expected safe fallback response, got %+v", fallback)
	}
}

func TestTestImpactAnalysisEndpointValidation(t *testing.T) {
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

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/release/tests/impact-analysis", bytes.NewReader([]byte(`{"changed_files":[]}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected validation error for empty changed files: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
