package control

import "testing"

func TestSSOProviderAndSessionFlow(t *testing.T) {
	store := NewIdentityStore()
	provider, err := store.CreateProvider(SSOProviderInput{
		Name:           "Okta",
		Protocol:       "oidc",
		IssuerURL:      "https://id.example.com",
		ClientID:       "masterchef-client",
		RedirectURL:    "https://masterchef.example.com/callback",
		AllowedDomains: []string{"example.com"},
	})
	if err != nil {
		t.Fatalf("create provider failed: %v", err)
	}
	if provider.ID == "" {
		t.Fatalf("expected provider id")
	}

	start, err := store.StartLogin(SSOLoginStartInput{
		ProviderID: provider.ID,
		Email:      "alice@example.com",
	})
	if err != nil {
		t.Fatalf("start login failed: %v", err)
	}
	if start.State == "" || start.AuthURL == "" {
		t.Fatalf("expected start login state/auth url")
	}

	session, err := store.CompleteLogin(SSOLoginCompleteInput{
		State:   start.State,
		Code:    "auth-code",
		Subject: "alice",
		Email:   "alice@example.com",
		Groups:  []string{"sre", "platform"},
	})
	if err != nil {
		t.Fatalf("complete login failed: %v", err)
	}
	if session.ID == "" || session.Token == "" {
		t.Fatalf("expected session id/token")
	}
	if _, ok := store.GetSession(session.ID); !ok {
		t.Fatalf("expected session lookup success")
	}
}

func TestSSODomainValidation(t *testing.T) {
	store := NewIdentityStore()
	provider, err := store.CreateProvider(SSOProviderInput{
		Name:           "Azure AD",
		Protocol:       "oidc",
		IssuerURL:      "https://login.example.com",
		ClientID:       "client-1",
		RedirectURL:    "https://masterchef.example.com/callback",
		AllowedDomains: []string{"example.com"},
	})
	if err != nil {
		t.Fatalf("create provider failed: %v", err)
	}
	if _, err := store.StartLogin(SSOLoginStartInput{
		ProviderID: provider.ID,
		Email:      "bob@other.com",
	}); err == nil {
		t.Fatalf("expected domain restriction failure")
	}
}
