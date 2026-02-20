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
	"time"
)

func TestActivityStreamEndpoint(t *testing.T) {
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

	seed := []byte(`{"type":"external.alert","message":"seeded replay event"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader(seed))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("seed ingest failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	streamCtx, cancel := context.WithCancel(context.Background())
	streamReq := httptest.NewRequest(http.MethodGet, "/v1/activity/stream?replay_limit=5&type_prefix=external.alert", nil).WithContext(streamCtx)
	streamRR := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		s.httpServer.Handler.ServeHTTP(streamRR, streamReq)
		close(done)
	}()

	time.Sleep(80 * time.Millisecond)
	live := []byte(`{"type":"external.alert","message":"live streamed event"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/events/ingest", bytes.NewReader(live))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("live ingest failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	time.Sleep(120 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for stream handler to exit")
	}

	body := streamRR.Body.String()
	if streamRR.Code != http.StatusOK {
		t.Fatalf("expected stream status 200, got %d body=%s", streamRR.Code, body)
	}
	if !strings.Contains(body, "event: activity") {
		t.Fatalf("expected SSE event frame, body=%s", body)
	}
	if !strings.Contains(body, "seeded replay event") || !strings.Contains(body, "live streamed event") {
		t.Fatalf("expected replay and live events in stream body, got %s", body)
	}
}
