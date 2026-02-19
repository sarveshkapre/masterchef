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

func TestComplianceEndpoints(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "c.yaml")
	features := filepath.Join(tmp, "features.md")
	if err := os.WriteFile(cfg, []byte(`version: v0
inventory:
  hosts:
    - name: localhost
      transport: local
resources:
  - id: marker
    type: file
    host: localhost
    path: `+filepath.Join(tmp, "marker.txt")+`
    content: "ok"
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

	createProfile := []byte(`{
  "name":"baseline-cis-linux",
  "framework":"cis",
  "version":"1.0.0",
  "controls":[
    {"id":"CIS-1.1","description":"filesystem perms","severity":"high"},
    {"id":"CIS-1.2","description":"integrity","severity":"medium"}
  ]
}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/compliance/profiles", bytes.NewReader(createProfile))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create profile failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var profile struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &profile)

	runScan := []byte(`{"profile_id":"` + profile.ID + `","target_kind":"host","target_name":"prod-1"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/compliance/scans", bytes.NewReader(runScan))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("run scan failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var scan struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &scan)

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/compliance/scans/"+scan.ID+"/evidence?format=csv", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "scan_id,profile_id") {
		t.Fatalf("csv evidence export failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/compliance/scans/"+scan.ID+"/evidence?format=sarif", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"version": "2.1.0"`) {
		t.Fatalf("sarif evidence export failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	continuous := []byte(`{"profile_id":"` + profile.ID + `","target_kind":"host","target_name":"prod-1","interval_seconds":300}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/compliance/continuous", bytes.NewReader(continuous))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create continuous config failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var cfgResp struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &cfgResp)

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/compliance/continuous/"+cfgResp.ID+"/run", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("run continuous scan failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
