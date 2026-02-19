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

func TestDiscoveryInventoryEndpoints(t *testing.T) {
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

	createBody := []byte(`{"name":"k8s-prod","kind":"kubernetes","endpoint":"https://kubernetes.default.svc","query":"namespace=prod","default_labels":{"team":"platform"},"enabled":true}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/inventory/discovery-sources", bytes.NewReader(createBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create discovery source failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var source struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &source)
	if source.ID == "" {
		t.Fatalf("expected source id")
	}

	syncBody := []byte(`{"source_id":"` + source.ID + `","hosts":[{"name":"pod-node-1","address":"10.1.0.1","transport":"ssh","labels":{"env":"prod"},"roles":["api"]},{"name":"pod-node-2","address":"10.1.0.2","transport":"ssh"}]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/inventory/discovery-sources/sync", bytes.NewReader(syncBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("discovery sync failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/inventory/runtime-hosts", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list runtime hosts failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"source":"discovery:kubernetes"`)) {
		t.Fatalf("expected discovery-enrolled hosts, body=%s", rr.Body.String())
	}
}

func TestCloudInventorySyncEndpoint(t *testing.T) {
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

	body := []byte(`{"provider":"aws","region":"us-east-1","hosts":[{"name":"ec2-1","address":"10.0.1.1","transport":"ssh","labels":{"service":"payments"}},{"name":"ec2-2","address":"10.0.1.2"}],"default_labels":{"team":"platform"}}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/inventory/cloud-sync", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("cloud sync failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"provider":"aws"`)) || !bytes.Contains(rr.Body.Bytes(), []byte(`"created":2`)) {
		t.Fatalf("unexpected cloud sync response: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/inventory/runtime-hosts", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list runtime hosts failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"source":"cloud:aws"`)) || !bytes.Contains(rr.Body.Bytes(), []byte(`"cloud_provider":"aws"`)) {
		t.Fatalf("expected cloud-synced hosts in runtime inventory: %s", rr.Body.String())
	}
}
