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

func TestEventStreamIngressAliases(t *testing.T) {
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
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })

	payload := []byte(`{"type":"external.alert","message":"from webhook","fields":{"sev":"high","source":"vendor"}}`)
	for _, path := range []string{
		"/v1/events/ingest",
		"/v1/event-stream/ingest",
		"/v1/event-stream/webhooks/ingest",
	} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
		s.httpServer.Handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusAccepted {
			t.Fatalf("ingest via %s failed: code=%d body=%s", path, rr.Code, rr.Body.String())
		}
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/activity?type_prefix=external.alert&limit=20", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("activity query failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Count(rr.Body.String(), `"type":"external.alert"`) < 3 {
		t.Fatalf("expected events from all alias ingests, got %s", rr.Body.String())
	}
}
