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

func TestSchedulerPartitionEndpoints(t *testing.T) {
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

	createBody := []byte(`{"tenant":"payments","environment":"prod","region":"us-east-1","shard":"shard-a","max_parallel":40}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/control/scheduler/partitions", bytes.NewReader(createBody))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create scheduler partition failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var rule struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &rule)
	if rule.ID == "" {
		t.Fatalf("expected rule id")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/control/scheduler/partition-decision", bytes.NewReader([]byte(`{"tenant":"payments","environment":"prod","region":"us-east-1","workload_key":"deploy/payments"}`)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("scheduler partition decision failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
