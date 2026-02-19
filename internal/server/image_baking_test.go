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

func TestImageBakePipelineEndpoints(t *testing.T) {
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

	createBody := []byte(`{
		"environment":"prod",
		"name":"linux-base",
		"builder":"packer",
		"base_image":"ubuntu-24.04",
		"target_image":"masterchef-linux-base",
		"artifact_format":"ami",
		"promote_after_bake":true,
		"hooks":[{"stage":"pre_bake","action":"validate-packages","required":true}]
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/execution/image-baking/pipelines", bytes.NewReader(createBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create image bake pipeline failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	var created struct {
		Pipeline struct {
			ID string `json:"id"`
		} `json:"pipeline"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}
	if created.Pipeline.ID == "" {
		t.Fatalf("expected created pipeline id in response: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/execution/image-baking/pipelines", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list image bake pipelines failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	planBody := []byte(`{"region":"us-west-2","build_ref":"git:abc123"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/execution/image-baking/pipelines/"+created.Pipeline.ID+"/plan", bytes.NewReader(planBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("plan image bake pipeline failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"promote-image"`)) {
		t.Fatalf("expected promote-image step in plan response: %s", rr.Body.String())
	}
}
