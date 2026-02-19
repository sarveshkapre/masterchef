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

func TestLeakDetectionEndpoints(t *testing.T) {
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

	policy := []byte(`{"min_samples":3,"memory_growth_percent":20,"goroutine_growth_percent":20,"file_descriptor_growth_percent":20}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/leak-detection/policy", bytes.NewReader(policy))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set leak policy failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	for _, body := range [][]byte{
		[]byte(`{"component":"scheduler","memory_mb":100,"goroutines":80,"open_fds":40}`),
		[]byte(`{"component":"scheduler","memory_mb":120,"goroutines":95,"open_fds":50}`),
		[]byte(`{"component":"scheduler","memory_mb":140,"goroutines":110,"open_fds":55}`),
	} {
		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/v1/control/leak-detection/snapshots", bytes.NewReader(body))
		s.httpServer.Handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("post leak snapshot failed: code=%d body=%s", rr.Code, rr.Body.String())
		}
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/leak-detection/reports", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list leak reports failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
