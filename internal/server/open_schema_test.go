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

func TestOpenSchemaEndpoints(t *testing.T) {
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

	create := []byte(`{"name":"masterchef-config","format":"json_schema","enabled":true,"content":"{\"type\":\"object\",\"required\":[\"version\",\"inventory\"]}"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/schema/models", bytes.NewReader(create))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create open schema failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var schema struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &schema); err != nil {
		t.Fatalf("decode schema response: %v", err)
	}

	badValidate := []byte(`{"schema_id":"` + schema.ID + `","document":"version: v0\n"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/schema/validate", bytes.NewReader(badValidate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected validation conflict for missing required field: code=%d body=%s", rr.Code, rr.Body.String())
	}

	goodValidate := []byte(`{"schema_id":"` + schema.ID + `","document":"version: v0\ninventory: {}\n"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/schema/validate", bytes.NewReader(goodValidate))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected validation success: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
