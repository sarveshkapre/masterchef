package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInterfaceCompatAnalyzeEndpoint(t *testing.T) {
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

	passBody := []byte(`{
		"baseline":{"kind":"module","name":"web-module","version":"v1","inputs":{"name":"string"},"outputs":{"status":"string"}},
		"current":{"kind":"module","name":"web-module","version":"v2","inputs":{"name":"string","region":"string"},"outputs":{"status":"string","endpoint":"string"}}
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/packages/interface-compat/analyze", bytes.NewReader(passBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected non-breaking report status 200: code=%d body=%s", rr.Code, rr.Body.String())
	}

	failBody := []byte(`{
		"baseline":{"kind":"provider","name":"pkg-provider","version":"v1","inputs":{"name":"string","version":"string"},"outputs":{"changed":"bool"}},
		"current":{"kind":"provider","name":"pkg-provider","version":"v2","inputs":{"name":"string"},"outputs":{"changed":"string"}}
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/packages/interface-compat/analyze", bytes.NewReader(failBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected breaking report conflict 409: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
