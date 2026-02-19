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

func TestCanaryUpgradeWorkflowWithAutoRollback(t *testing.T) {
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

	// Seed current channel.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/channels", bytes.NewReader([]byte(`{"action":"set_channel","component":"control-plane","channel":"stable"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set channel failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	// Success path (no regression detected).
	upgradeOK := []byte(`{"component":"control-plane","to_channel":"candidate","auto_rollback":true}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/canary-upgrades", bytes.NewReader(upgradeOK))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("canary upgrade success path failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var okRun struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		RolledBack bool   `json:"rolled_back"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &okRun)
	if okRun.ID == "" || okRun.Status != "completed" || okRun.RolledBack {
		t.Fatalf("unexpected success run: %+v", okRun)
	}

	// Regression path triggers automatic rollback.
	upgradeBad := []byte(`{"component":"control-plane","to_channel":"edge","auto_rollback":true,"canary_ids":["missing-canary"]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/canary-upgrades", bytes.NewReader(upgradeBad))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected rollback conflict, got code=%d body=%s", rr.Code, rr.Body.String())
	}
	var rollbackRun struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		RolledBack  bool   `json:"rolled_back"`
		FromChannel string `json:"from_channel"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &rollbackRun)
	if rollbackRun.Status != "rolled_back" || !rollbackRun.RolledBack {
		t.Fatalf("expected rolled back run, got %+v", rollbackRun)
	}

	// Component channel should be rolled back to previous channel.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/channels?control_plane_protocol=5", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get channels failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"component":"control-plane"`)) || !bytes.Contains(rr.Body.Bytes(), []byte(`"channel":"candidate"`)) {
		t.Fatalf("expected channel rollback to previous value, body=%s", rr.Body.String())
	}

	// List/get runs endpoints.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/canary-upgrades?limit=10", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list canary upgrades failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/canary-upgrades/"+rollbackRun.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get canary upgrade run failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
