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

func TestNetworkTransportEndpoints(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/v1/execution/network-transports", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list network transports failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"name":"netconf"`) {
		t.Fatalf("expected builtin netconf transport: %s", rr.Body.String())
	}

	register := []byte(`{"name":"eapi","description":"Arista eAPI","category":"api","default_port":443,"supports_config_push":true,"supports_telemetry_pull":true,"credential_fields_required":["endpoint","token"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/network-transports", bytes.NewReader(register))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("register network transport failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	validate := []byte(`{"transport":"eapi"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/network-transports/validate", bytes.NewReader(validate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("validate supported transport failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"supported":true`) {
		t.Fatalf("expected supported=true in validation response: %s", rr.Body.String())
	}

	unsupported := []byte(`{"transport":"snmp"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/network-transports/validate", bytes.NewReader(unsupported))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("validate unsupported transport expected conflict: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
