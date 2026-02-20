package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanDiffPreviewEndpoint(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target.txt")
	cfg := filepath.Join(tmp, "diff.yaml")
	features := filepath.Join(tmp, "features.md")
	if err := os.WriteFile(target, []byte("old\nline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: update-file
    type: file
    host: localhost
    path: `+target+`
    content: "new\nline\n"
  - id: restart-service
    type: command
    host: localhost
    command: "echo restart"
    subscribe: ["update-file"]
    refresh_command: "echo reload"
    refresh_only: true
    only_if: "test -f /tmp/reload-ready"
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
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	body := []byte(`{"config_path":"diff.yaml"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/plans/diff-preview", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("plan diff preview failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"changed_actions":2`) {
		t.Fatalf("expected changed_actions=2: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `--- current`) || !strings.Contains(rr.Body.String(), `+++ desired`) {
		t.Fatalf("expected inline diff markers: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `refresh_only: true`) || !strings.Contains(rr.Body.String(), `only_if guard`) {
		t.Fatalf("expected command refresh/guard markers in preview: %s", rr.Body.String())
	}

	patchBody := []byte(`{"config_path":"diff.yaml","format":"patch"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/plans/diff-preview", bytes.NewReader(patchBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("plan diff patch format failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"format":"patch"`) {
		t.Fatalf("expected patch format in response: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"op":"replace"`) || !strings.Contains(rr.Body.String(), `"op":"execute"`) {
		t.Fatalf("expected machine-readable patch ops in response: %s", rr.Body.String())
	}
}
