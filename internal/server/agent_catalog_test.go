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

	"github.com/masterchef/masterchef/internal/policy"
)

func TestAgentCatalogEndpoints(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "catalog.yaml")
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

	pub, priv, err := policy.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate keypair failed: %v", err)
	}
	if err := policy.SavePrivateKey(filepath.Join(tmp, "catalog-private.key"), priv); err != nil {
		t.Fatalf("save private key failed: %v", err)
	}
	if err := policy.SavePublicKey(filepath.Join(tmp, "catalog-public.key"), pub); err != nil {
		t.Fatalf("save public key failed: %v", err)
	}

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	compilePayload := []byte(`{"config_path":"catalog.yaml","policy_group":"stable","agent_ids":["agent-a"],"private_key_path":"catalog-private.key"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/catalogs", bytes.NewReader(compilePayload))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("compile catalog failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var catalog struct {
		ID     string `json:"id"`
		Signed bool   `json:"signed"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &catalog)
	if catalog.ID == "" || !catalog.Signed {
		t.Fatalf("expected signed catalog, got %+v", catalog)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/agents/catalogs/"+catalog.ID, nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get catalog failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	replayWrongAgent := []byte(`{"catalog_id":"` + catalog.ID + `","agent_id":"agent-b","public_key_path":"catalog-public.key","disconnected":true}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/catalogs/replay", bytes.NewReader(replayWrongAgent))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected replay to fail for untargeted agent: code=%d body=%s", rr.Code, rr.Body.String())
	}

	replayPayload := []byte(`{"catalog_id":"` + catalog.ID + `","agent_id":"agent-a","public_key_path":"catalog-public.key","disconnected":true}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/catalogs/replay", bytes.NewReader(replayPayload))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("replay signed catalog failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var replay struct {
		Allowed  bool `json:"allowed"`
		Verified bool `json:"verified"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &replay)
	if !replay.Allowed || !replay.Verified {
		t.Fatalf("expected allowed verified replay, got %+v", replay)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/agents/catalogs/replays?limit=10", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list catalog replays failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
