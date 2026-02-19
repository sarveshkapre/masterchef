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
	"time"
)

func TestRunLeaseEndpoints(t *testing.T) {
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

	job, err := s.queue.Enqueue("lease-target.yaml", "", true, "")
	if err != nil {
		t.Fatalf("enqueue job for lease test: %v", err)
	}

	acquire := []byte(`{"job_id":"` + job.ID + `","holder":"worker-1","ttl_seconds":1}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/run-leases", bytes.NewReader(acquire))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("acquire run lease failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var lease struct {
		LeaseID string `json:"lease_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &lease); err != nil {
		t.Fatalf("decode lease acquire: %v", err)
	}
	if lease.LeaseID == "" {
		t.Fatalf("expected lease id in acquire response")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/control/run-leases", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), lease.LeaseID) {
		t.Fatalf("list leases failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	heartbeat := []byte(`{"lease_id":"` + lease.LeaseID + `"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/run-leases/heartbeat", bytes.NewReader(heartbeat))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("heartbeat lease failed: code=%d body=%s", rr.Code, rr.Body.String())
	}

	recoverBody := []byte(`{"now":"` + time.Now().UTC().Add(2*time.Minute).Format(time.RFC3339) + `"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/run-leases/recover", bytes.NewReader(recoverBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("recover leases failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"recovered_count":1`) || !strings.Contains(rr.Body.String(), `"status":"failed"`) {
		t.Fatalf("expected one recovered lease and failed job, got: %s", rr.Body.String())
	}
}
