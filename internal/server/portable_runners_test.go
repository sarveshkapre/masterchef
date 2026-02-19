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

func TestPortableRunnerEndpoints(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/v1/execution/portable-runners", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list portable runners failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"name":"posix-static-runner"`) {
		t.Fatalf("expected builtin runner profile: %s", rr.Body.String())
	}

	register := []byte(`{
  "name":"linux-s390x-runner",
  "os_families":["linux"],
  "architectures":["s390x"],
  "transport_modes":["ssh"],
  "shell":"sh",
  "artifact_ref":"runner://linux-s390x",
  "checksum_sha256":"sha256:3333333333333333333333333333333333333333333333333333333333333333",
  "supports_no_python":true
}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/portable-runners", bytes.NewReader(register))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("register portable runner failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	selectReq := []byte(`{"os_family":"linux","architecture":"s390x","transport_mode":"ssh"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/portable-runners/select", bytes.NewReader(selectReq))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("select portable runner failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"supported":true`) || !strings.Contains(rr.Body.String(), `"mode":"pythonless-portable"`) {
		t.Fatalf("expected supported pythonless mode: %s", rr.Body.String())
	}

	pythonRequired := []byte(`{"os_family":"linux","architecture":"amd64","transport_mode":"ssh","python_required":true}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/portable-runners/select", bytes.NewReader(pythonRequired))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("python-required selection expected conflict: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
