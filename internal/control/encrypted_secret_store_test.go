package control

import (
	"testing"
	"time"
)

func TestEncryptedSecretUpsertResolveRotate(t *testing.T) {
	store := NewEncryptedSecretStore()
	base := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	store.now = func() time.Time { return base }

	item, err := store.Upsert(EncryptedSecretUpsertInput{
		Name:       "db_password",
		Value:      "s3cr3t-v1",
		TTLSeconds: 3600,
		Labels:     map[string]string{"service": "payments"},
	})
	if err != nil {
		t.Fatalf("upsert encrypted secret failed: %v", err)
	}
	if item.Version != 1 {
		t.Fatalf("expected version 1 on first upsert, got %+v", item)
	}

	resolved, err := store.Resolve("db_password")
	if err != nil {
		t.Fatalf("resolve encrypted secret failed: %v", err)
	}
	if resolved.Value != "s3cr3t-v1" {
		t.Fatalf("unexpected secret plaintext, got %+v", resolved)
	}

	store.now = func() time.Time { return base.Add(30 * time.Minute) }
	rotated, err := store.Rotate("db_password", EncryptedSecretRotateInput{
		Value:            "s3cr3t-v2",
		ExtendTTLSeconds: 7200,
	})
	if err != nil {
		t.Fatalf("rotate encrypted secret failed: %v", err)
	}
	if rotated.Version != 2 || rotated.RotationCount != 1 {
		t.Fatalf("expected rotated metadata update, got %+v", rotated)
	}

	resolved, err = store.Resolve("db_password")
	if err != nil {
		t.Fatalf("resolve rotated encrypted secret failed: %v", err)
	}
	if resolved.Value != "s3cr3t-v2" {
		t.Fatalf("unexpected rotated plaintext, got %+v", resolved)
	}
}

func TestEncryptedSecretExpiryEnforcement(t *testing.T) {
	store := NewEncryptedSecretStore()
	base := time.Date(2026, time.February, 1, 9, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return base }
	if _, err := store.Upsert(EncryptedSecretUpsertInput{
		Name:       "api_token",
		Value:      "token-v1",
		TTLSeconds: 10,
	}); err != nil {
		t.Fatalf("upsert encrypted secret failed: %v", err)
	}

	store.now = func() time.Time { return base.Add(11 * time.Second) }
	if _, err := store.Resolve("api_token"); err == nil {
		t.Fatalf("expected resolve to fail after expiry")
	}
	expired := store.Expired()
	if len(expired) != 1 || expired[0].Name != "api_token" {
		t.Fatalf("expected expired item listing, got %+v", expired)
	}
}
