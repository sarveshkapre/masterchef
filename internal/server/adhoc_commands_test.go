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

func TestAdHocCommandEndpoints(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/v1/commands/adhoc/policy", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get adhoc policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"blocked_patterns"`) {
		t.Fatalf("expected policy in response: %s", rr.Body.String())
	}

	blocked := []byte(`{"command":"rm -rf /tmp/demo","requested_by":"sre","reason":"test","dry_run":true}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/commands/adhoc", bytes.NewReader(blocked))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected guardrail block conflict: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"blocked"`) {
		t.Fatalf("expected blocked status in response: %s", rr.Body.String())
	}

	dryRun := []byte(`{"command":"echo dry-run-ok","requested_by":"sre","reason":"diagnostic","dry_run":true}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/commands/adhoc", bytes.NewReader(dryRun))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("dry-run adhoc failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"approved"`) {
		t.Fatalf("expected approved status for dry-run: %s", rr.Body.String())
	}

	execute := []byte(`{"command":"printf run-ok","requested_by":"sre","reason":"diagnostic","dry_run":false}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/commands/adhoc", bytes.NewReader(execute))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("execute adhoc failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"succeeded"`) || !strings.Contains(rr.Body.String(), `run-ok`) {
		t.Fatalf("expected succeeded execution with output, body=%s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/commands/adhoc?limit=10", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list adhoc history failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `adhoc-`) {
		t.Fatalf("expected adhoc history entries in response: %s", rr.Body.String())
	}
}
