package control

import "testing"

func TestTenantCryptoStoreLifecycleAndBoundaryCheck(t *testing.T) {
	store := NewTenantCryptoStore()
	key, err := store.EnsureTenantKey(TenantCryptoKeyInput{
		Tenant:    "tenant-a",
		Algorithm: "aes-256-gcm",
	})
	if err != nil {
		t.Fatalf("ensure tenant key failed: %v", err)
	}
	if key.ID == "" || key.Version != 1 {
		t.Fatalf("unexpected key %+v", key)
	}

	rotated, err := store.Rotate(TenantKeyRotateInput{Tenant: "tenant-a"})
	if err != nil {
		t.Fatalf("rotate tenant key failed: %v", err)
	}
	if rotated.Version != 2 || rotated.ID == key.ID {
		t.Fatalf("expected rotated key version 2, got %+v", rotated)
	}

	allowed := store.BoundaryCheck(TenantBoundaryCheckInput{
		RequestTenant: "tenant-a",
		ContextTenant: "tenant-a",
		KeyID:         rotated.ID,
		Operation:     "decrypt",
	})
	if !allowed.Allowed {
		t.Fatalf("expected allowed boundary check, got %+v", allowed)
	}

	crossTenant := store.BoundaryCheck(TenantBoundaryCheckInput{
		RequestTenant: "tenant-a",
		ContextTenant: "tenant-b",
		KeyID:         rotated.ID,
	})
	if crossTenant.Allowed {
		t.Fatalf("expected cross-tenant rejection, got %+v", crossTenant)
	}

	wrongOwner := store.BoundaryCheck(TenantBoundaryCheckInput{
		RequestTenant: "tenant-b",
		ContextTenant: "tenant-b",
		KeyID:         rotated.ID,
	})
	if wrongOwner.Allowed {
		t.Fatalf("expected wrong-owner rejection, got %+v", wrongOwner)
	}
}

func TestTenantCryptoStoreValidation(t *testing.T) {
	store := NewTenantCryptoStore()
	if _, err := store.EnsureTenantKey(TenantCryptoKeyInput{
		Tenant:    "tenant-a",
		Algorithm: "rsa-4096",
	}); err == nil {
		t.Fatalf("expected invalid algorithm error")
	}
	if _, err := store.Rotate(TenantKeyRotateInput{Tenant: "missing"}); err == nil {
		t.Fatalf("expected missing key rotate error")
	}
}
