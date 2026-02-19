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

func TestDocsGenerateEndpoint(t *testing.T) {
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

	publish := []byte(`{
  "kind":"module",
  "name":"core/network",
  "version":"1.0.0",
  "digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111",
  "signed":true,
  "key_id":"sigkey-1",
  "signature":"sig"
}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/packages/artifacts", bytes.NewReader(publish))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("publish package artifact failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/docs/generate", bytes.NewReader([]byte(`{"format":"markdown","include_packages":true,"include_policy_api":true}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("docs generate failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "core/network@1.0.0") {
		t.Fatalf("expected package docs in generated artifact: %s", body)
	}
	if !strings.Contains(body, "/v1/policy/pull/sources") {
		t.Fatalf("expected policy api docs in generated artifact: %s", body)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/docs/examples/verify", bytes.NewReader([]byte(`{}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("docs examples verify failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
