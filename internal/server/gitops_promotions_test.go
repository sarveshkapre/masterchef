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

func TestGitOpsPromotionEndpoints(t *testing.T) {
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

	createBody := []byte(`{
  "name":"service-a",
  "stages":["staging","canary","production"],
  "artifact_digest":"sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
  "actor":"release-bot"
}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/gitops/promotions", bytes.NewReader(createBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create promotion failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"current_stage":"staging"`) {
		t.Fatalf("expected initial staging stage: %s", body)
	}
	idIdx := strings.Index(body, `"id":"`)
	if idIdx < 0 {
		t.Fatalf("expected promotion id in body: %s", body)
	}
	idStart := idIdx + len(`"id":"`)
	idEnd := strings.Index(body[idStart:], `"`)
	if idEnd < 0 {
		t.Fatalf("unable to parse promotion id: %s", body)
	}
	promotionID := body[idStart : idStart+idEnd]

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/gitops/promotions/"+promotionID+"/advance", bytes.NewReader([]byte(`{"artifact_digest":"sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected immutable digest conflict, code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/gitops/promotions/"+promotionID+"/advance", bytes.NewReader([]byte(`{"artifact_digest":"sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","actor":"release-bot"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"current_stage":"canary"`) {
		t.Fatalf("advance to canary failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/gitops/promotions/"+promotionID+"/advance", bytes.NewReader([]byte(`{"artifact_digest":"sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","actor":"release-bot"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"status":"completed"`) {
		t.Fatalf("advance to completed failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
