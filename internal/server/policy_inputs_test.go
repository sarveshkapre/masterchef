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

func TestPolicyInputResolveEndpoint(t *testing.T) {
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
	t.Setenv("MC_POLICY_REGION", "us-east-1")

	filePath := filepath.Join(tmp, "policy-inputs.json")
	if err := os.WriteFile(filePath, []byte(`{"policy":{"owner":"platform"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	body := []byte(`{
		"strategy":"merge-last",
		"sources":[
			{"name":"inline","type":"inline","config":{"data":{"policy":{"owner":"apps","enforce":true}}}},
			{"name":"env","type":"env","config":{"prefix":"MC_POLICY_","target":"runtime"}},
			{"name":"file","type":"file","config":{"path":"policy-inputs.json"}}
		]
	}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/policy/inputs/resolve", bytes.NewReader(body))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("policy input resolve failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	payload := rr.Body.String()
	if !strings.Contains(payload, `"resolved_from":3`) {
		t.Fatalf("expected resolved_from count in response: %s", payload)
	}
	if !strings.Contains(payload, `"region":"us-east-1"`) {
		t.Fatalf("expected env input in response: %s", payload)
	}
	if !strings.Contains(payload, `"owner":"platform"`) {
		t.Fatalf("expected file source to override owner with merge-last: %s", payload)
	}

	conflict := []byte(`{
		"hard_fail":true,
		"sources":[
			{"name":"a","type":"inline","config":{"data":{"policy":{"mode":"audit"}}}},
			{"name":"b","type":"inline","config":{"data":{"policy":{"mode":"enforce"}}}}
		]
	}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/policy/inputs/resolve", bytes.NewReader(conflict))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected hard-fail conflict status, got code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"conflicts"`) {
		t.Fatalf("expected conflicts in hard-fail response: %s", rr.Body.String())
	}
}
