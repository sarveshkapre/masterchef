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

func TestMigrationToolingEndpoints(t *testing.T) {
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

	translate := []byte(`{"source_platform":"ansible","source_content":"- hosts: all\n  tasks:\n    - apt: name=nginx state=present\n  become: true","workload":"web"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/migrations/translate", bytes.NewReader(translate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("migration translate failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode migration translate response failed: %v", err)
	}
	translationID, _ := created["id"].(string)
	if translationID == "" {
		t.Fatalf("expected translation id in response: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/migrations/translations", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list migration translations failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	eq := []byte(`{"translation_id":"` + translationID + `","semantic_checks":[{"name":"playbook","expected":"playbooks","translated":"playbooks"}]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/migrations/equivalence-check", bytes.NewReader(eq))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("migration equivalence check failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	diff := []byte(`{"translation_id":"` + translationID + `"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/migrations/diff-report", bytes.NewReader(diff))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("migration diff report failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
