package control

import "testing"

func TestSecretsIntegrationResolveAndTrace(t *testing.T) {
	store := NewSecretsIntegrationStore()
	item, err := store.Upsert(SecretsIntegrationInput{
		Name:     "vault-prod",
		Provider: "inline",
		Config: map[string]string{
			"secret/db/password": "wrong-format-ignored",
			"secret.db/password": "super-secret",
			"secret/api/key":     "token123",
		},
	})
	if err != nil {
		t.Fatalf("upsert integration failed: %v", err)
	}
	if item.ID == "" {
		t.Fatalf("expected integration id")
	}

	_, err = store.Resolve(SecretResolveInput{
		IntegrationID: item.ID,
		Path:          "db/password",
		UsedBy:        "run-123",
	})
	if err != nil {
		t.Fatalf("resolve secret failed: %v", err)
	}

	traces := store.ListUsageTraces(10)
	if len(traces) != 1 {
		t.Fatalf("expected one usage trace, got %d", len(traces))
	}
	if traces[0].RedactedValue != "<redacted>" {
		t.Fatalf("expected redacted trace value, got %+v", traces[0])
	}
}
