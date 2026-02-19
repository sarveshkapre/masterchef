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

func TestProviderProtocolEndpoints(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/v1/providers/protocol/descriptors", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list provider protocol descriptors failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	upsert := []byte(`{"id":"provider-protocol-service-v1","provider":"service","protocol_version":"v1.0","min_controller_version":"v1.0","max_controller_version":"v2.0","capabilities":["restart","health-gate"],"feature_flags":{"canary-eval":true}}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/providers/protocol/descriptors", bytes.NewReader(upsert))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upsert provider protocol descriptor failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	okNegotiate := []byte(`{"provider":"service","controller_version":"v1.2","requested_capabilities":["restart"],"required_feature_flags":["canary-eval"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/providers/protocol/negotiate", bytes.NewReader(okNegotiate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected compatible negotiation status 200: code=%d body=%s", rr.Code, rr.Body.String())
	}

	badNegotiate := []byte(`{"provider":"service","controller_version":"v1.2","requested_capabilities":["not-supported"],"required_feature_flags":["missing-flag"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/providers/protocol/negotiate", bytes.NewReader(badNegotiate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected incompatible negotiation status 409: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
