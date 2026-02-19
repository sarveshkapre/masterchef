package control

import "testing"

func TestOIDCWorkloadProviderAndExchange(t *testing.T) {
	store := NewOIDCWorkloadStore()
	provider, err := store.CreateProvider(OIDCWorkloadProviderInput{
		Name:      "aws-oidc",
		IssuerURL: "https://issuer.example.com",
		Audience:  "masterchef",
		JWKSURL:   "https://issuer.example.com/.well-known/jwks.json",
		AllowedSA: []string{"sa-prod-deployer"},
	})
	if err != nil {
		t.Fatalf("create provider failed: %v", err)
	}
	if provider.ID == "" {
		t.Fatalf("expected provider id")
	}

	cred, err := store.Exchange(OIDCWorkloadExchangeInput{
		ProviderID:     provider.ID,
		SubjectToken:   "id-token",
		ServiceAccount: "sa-prod-deployer",
		Workload:       "payments-api",
	})
	if err != nil {
		t.Fatalf("exchange failed: %v", err)
	}
	if cred.ID == "" || cred.Token == "" {
		t.Fatalf("expected credential id/token")
	}
}

func TestOIDCWorkloadExchangePolicyFailure(t *testing.T) {
	store := NewOIDCWorkloadStore()
	provider, err := store.CreateProvider(OIDCWorkloadProviderInput{
		Name:      "gcp-oidc",
		IssuerURL: "https://issuer.example.com",
		Audience:  "masterchef",
		JWKSURL:   "https://issuer.example.com/keys",
		AllowedSA: []string{"sa-allowed"},
	})
	if err != nil {
		t.Fatalf("create provider failed: %v", err)
	}
	if _, err := store.Exchange(OIDCWorkloadExchangeInput{
		ProviderID:     provider.ID,
		SubjectToken:   "id-token",
		ServiceAccount: "sa-blocked",
		Workload:       "payments-api",
	}); err == nil {
		t.Fatalf("expected exchange rejection for disallowed service account")
	}
}
