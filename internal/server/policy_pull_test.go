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

func TestPolicyPullEndpoints(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "policy.yaml")
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

	cpBody := []byte(`{"name":"cp","type":"control_plane","enabled":true}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/policy/pull/sources", bytes.NewReader(cpBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create control-plane source failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var cpSource struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &cpSource)
	if cpSource.ID == "" {
		t.Fatalf("expected control-plane source id")
	}

	execBody := []byte(`{"source_id":"` + cpSource.ID + `","config_path":"policy.yaml"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/policy/pull/execute", bytes.NewReader(execBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("execute control-plane pull failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var cpResult struct {
		Status   string `json:"status"`
		Verified bool   `json:"verified"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &cpResult)
	if cpResult.Status != "pulled" || !cpResult.Verified {
		t.Fatalf("unexpected control-plane pull result %+v", cpResult)
	}

	pub, priv, err := policy.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate keypair failed: %v", err)
	}
	if err := policy.SavePublicKey(filepath.Join(tmp, "policy-public.key"), pub); err != nil {
		t.Fatalf("save public key failed: %v", err)
	}

	gitSourceBody := []byte(`{"name":"git","type":"git_signed","url":"https://example.com/config.git","branch":"main","public_key_path":"policy-public.key","require_signature":true,"enabled":true}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/policy/pull/sources", bytes.NewReader(gitSourceBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create git source failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var gitSource struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &gitSource)
	if gitSource.ID == "" {
		t.Fatalf("expected git source id")
	}

	bundle, err := policy.Build(cfg)
	if err != nil {
		t.Fatalf("build bundle failed: %v", err)
	}
	if err := bundle.Sign(priv); err != nil {
		t.Fatalf("sign bundle failed: %v", err)
	}

	goodPayload, err := json.Marshal(map[string]any{
		"source_id": gitSource.ID,
		"revision":  "abc123",
		"bundle": map[string]any{
			"config_path": bundle.ConfigPath,
			"config_sha":  bundle.ConfigSHA,
			"signature":   bundle.Signature,
		},
	})
	if err != nil {
		t.Fatalf("marshal good payload failed: %v", err)
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/policy/pull/execute", bytes.NewReader(goodPayload))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("execute signed git pull failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var gitResult struct {
		Verified bool   `json:"verified"`
		Status   string `json:"status"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &gitResult)
	if !gitResult.Verified || gitResult.Status != "pulled" {
		t.Fatalf("unexpected signed git pull result %+v", gitResult)
	}

	badPayload, err := json.Marshal(map[string]any{
		"source_id": gitSource.ID,
		"revision":  "def456",
		"bundle": map[string]any{
			"config_path": bundle.ConfigPath,
			"config_sha":  "invalid-sha",
			"signature":   bundle.Signature,
		},
	})
	if err != nil {
		t.Fatalf("marshal bad payload failed: %v", err)
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/policy/pull/execute", bytes.NewReader(badPayload))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected signature failure conflict: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/policy/pull/results?limit=10", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list pull results failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
