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

func TestVariableSourceResolveEndpoint(t *testing.T) {
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
	t.Setenv("MC_SRC_REGION", "us-east-1")

	filePath := filepath.Join(tmp, "vars.json")
	if err := os.WriteFile(filePath, []byte(`{"service":{"name":"checkout"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	httpSource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"service":{"replicas":5}}`))
	}))
	defer httpSource.Close()

	s := New(":0", tmp)
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
	})

	body := `{
		"sources":[
			{"name":"env","type":"env","config":{"prefix":"MC_SRC_","target":"runtime"}},
			{"name":"file","type":"file","config":{"path":"vars.json"}},
			{"name":"http","type":"http","config":{"url":"` + httpSource.URL + `"}}
		],
		"layers":[{"name":"base","data":{"service":{"replicas":2}}}]
	}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/vars/sources/resolve", bytes.NewReader([]byte(body)))
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("vars source resolve failed: code=%d body=%s", rr.Code, rr.Body.String())
	}
	resp := rr.Body.String()
	if !strings.Contains(resp, `"total_layers":4`) {
		t.Fatalf("expected four layers in response: %s", resp)
	}
	if !strings.Contains(resp, `"replicas":5`) {
		t.Fatalf("expected http source precedence in merged output: %s", resp)
	}
	if !strings.Contains(resp, `"region":"us-east-1"`) {
		t.Fatalf("expected env source value in merged output: %s", resp)
	}
}
