package control

import "testing"

func TestDataBagStorePlainCRUD(t *testing.T) {
	store := NewDataBagStore()

	created, err := store.Upsert("Apps", "Payments", map[string]any{
		"owner": "sre",
		"tier":  "critical",
	}, false, "", []string{"prod", "payments"})
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	if created.Encrypted {
		t.Fatalf("expected plaintext item")
	}
	if created.Bag != "apps" || created.Item != "payments" {
		t.Fatalf("expected normalized names, got %#v", created)
	}

	got, err := store.Get("apps", "payments", "")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Data["owner"] != "sre" {
		t.Fatalf("unexpected get payload: %#v", got.Data)
	}

	summaries := store.ListSummaries()
	if len(summaries) != 1 {
		t.Fatalf("expected one summary, got %d", len(summaries))
	}
	if summaries[0].Data["tier"] != "critical" {
		t.Fatalf("expected plaintext summary payload, got %#v", summaries[0].Data)
	}

	if !store.Delete("apps", "payments") {
		t.Fatalf("expected delete success")
	}
	if _, err := store.Get("apps", "payments", ""); err == nil {
		t.Fatalf("expected not found after delete")
	}
}

func TestDataBagStoreEncryptedRoundTrip(t *testing.T) {
	store := NewDataBagStore()
	passphrase := "top-secret"

	created, err := store.Upsert("secrets", "db", map[string]any{
		"username": "app",
		"password": "p@ss",
		"nested": map[string]any{
			"port": 5432,
		},
	}, true, passphrase, []string{"db"})
	if err != nil {
		t.Fatalf("encrypted upsert failed: %v", err)
	}
	if !created.Encrypted || created.Ciphertext == "" || created.Nonce == "" {
		t.Fatalf("expected encrypted payload fields, got %#v", created)
	}
	if len(created.Data) != 0 {
		t.Fatalf("expected no plaintext in encrypted record response")
	}

	if _, err := store.Get("secrets", "db", ""); err == nil {
		t.Fatalf("expected passphrase requirement")
	}
	if _, err := store.Get("secrets", "db", "wrong"); err == nil {
		t.Fatalf("expected decrypt failure with wrong passphrase")
	}
	got, err := store.Get("secrets", "db", passphrase)
	if err != nil {
		t.Fatalf("encrypted get failed: %v", err)
	}
	if got.Data["username"] != "app" {
		t.Fatalf("unexpected decrypted payload: %#v", got.Data)
	}

	summaries := store.ListSummaries()
	if len(summaries) != 1 {
		t.Fatalf("expected one summary, got %d", len(summaries))
	}
	if summaries[0].Data != nil {
		t.Fatalf("expected encrypted summary to hide plaintext data")
	}
}

func TestDataBagStoreStructuredSearch(t *testing.T) {
	store := NewDataBagStore()
	passphrase := "vault-pass"

	_, _ = store.Upsert("apps", "payments", map[string]any{
		"owner": "sre-payments",
		"meta": map[string]any{
			"team": "platform-payments",
		},
	}, false, "", nil)
	_, _ = store.Upsert("apps", "search", map[string]any{
		"owner": "sre-search",
		"meta": map[string]any{
			"team": "platform-search",
		},
	}, false, "", nil)
	_, _ = store.Upsert("apps", "api-secrets", map[string]any{
		"token": "abc123",
		"meta": map[string]any{
			"team": "platform-payments",
		},
	}, true, passphrase, nil)

	result, err := store.Search(DataBagSearchRequest{
		Bag:      "apps",
		Field:    "meta.team",
		Contains: "payments",
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(result) != 1 || result[0].Item != "payments" {
		t.Fatalf("unexpected plaintext search result: %#v", result)
	}

	result, err = store.Search(DataBagSearchRequest{
		Bag:        "apps",
		Field:      "meta.team",
		Equals:     "platform-payments",
		Passphrase: passphrase,
	})
	if err != nil {
		t.Fatalf("encrypted search failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected encrypted+plaintext match, got %#v", result)
	}
	if result[0].Data == nil || result[1].Data == nil {
		t.Fatalf("expected search response to include data for matches")
	}
}
