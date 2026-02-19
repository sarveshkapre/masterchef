package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestENCProviderStoreClassify(t *testing.T) {
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := map[string]any{
			"classes":     []string{"base", "web"},
			"run_list":    []string{"role[web]", "recipe[nginx]"},
			"environment": "prod",
			"attributes": map[string]any{
				"team": "platform",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer h.Close()

	store := NewENCProviderStore()
	provider, err := store.Upsert(ENCProviderInput{
		Name:     "enc-http",
		Endpoint: h.URL,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("upsert provider: %v", err)
	}
	out, err := store.Classify(ENCClassifyInput{
		ProviderID: provider.ID,
		Node:       "web-1",
		Facts:      map[string]any{"os": "linux"},
		Labels:     map[string]any{"env": "prod"},
	})
	if err != nil {
		t.Fatalf("classify via enc provider: %v", err)
	}
	if len(out.Classes) != 2 || out.Environment != "prod" {
		t.Fatalf("unexpected classify output %+v", out)
	}
}

func TestENCProviderDisabled(t *testing.T) {
	store := NewENCProviderStore()
	provider, err := store.Upsert(ENCProviderInput{
		Name:     "enc-disabled",
		Endpoint: "https://example.invalid/enc",
		Enabled:  false,
	})
	if err != nil {
		t.Fatalf("upsert provider: %v", err)
	}
	if _, err := store.Classify(ENCClassifyInput{ProviderID: provider.ID, Node: "x"}); err == nil {
		t.Fatalf("expected classify with disabled provider to fail")
	}
}
