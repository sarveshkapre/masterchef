package control

import "testing"

func TestDelegatedAdminStoreCreateAndAuthorize(t *testing.T) {
	store := NewDelegatedAdminStore()
	grant, err := store.Create(DelegatedAdminGrantInput{
		Tenant:      "tenant-a",
		Environment: "prod",
		Principal:   "sre:oncall",
		Scopes:      []string{"runs.cancel", "workflows.*"},
		Delegator:   "platform-admin",
	})
	if err != nil {
		t.Fatalf("create delegated admin grant failed: %v", err)
	}
	if grant.ID == "" {
		t.Fatalf("expected grant id")
	}

	allowExact := store.Authorize(DelegatedAdminAuthorizeInput{
		Tenant:      "tenant-a",
		Environment: "prod",
		Principal:   "sre:oncall",
		Action:      "runs.cancel",
	})
	if !allowExact.Allowed {
		t.Fatalf("expected exact-scope authorization, got %+v", allowExact)
	}

	allowWildcard := store.Authorize(DelegatedAdminAuthorizeInput{
		Tenant:      "tenant-a",
		Environment: "prod",
		Principal:   "sre:oncall",
		Action:      "workflows.launch",
	})
	if !allowWildcard.Allowed {
		t.Fatalf("expected wildcard scope authorization, got %+v", allowWildcard)
	}

	deny := store.Authorize(DelegatedAdminAuthorizeInput{
		Tenant:      "tenant-a",
		Environment: "prod",
		Principal:   "sre:oncall",
		Action:      "packages.publish",
	})
	if deny.Allowed {
		t.Fatalf("expected deny for ungranted action, got %+v", deny)
	}
}

func TestDelegatedAdminStoreValidation(t *testing.T) {
	store := NewDelegatedAdminStore()
	if _, err := store.Create(DelegatedAdminGrantInput{
		Tenant:      "tenant-a",
		Environment: "prod",
		Principal:   "sre:oncall",
	}); err == nil {
		t.Fatalf("expected missing scopes validation error")
	}
}
