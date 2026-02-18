package control

import "testing"

func TestEncryptedVariableStoreUpsertGetRotate(t *testing.T) {
	baseDir := t.TempDir()
	store := NewEncryptedVariableStore(baseDir)

	summary, err := store.Upsert("prod-vars", map[string]any{
		"db_user": "svc",
		"db_pass": "secret",
	}, "pass-v1")
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	if summary.KeyVersion != 1 {
		t.Fatalf("expected key version 1, got %d", summary.KeyVersion)
	}

	data, meta, err := store.Get("prod-vars", "pass-v1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if meta.KeyVersion != 1 || data["db_user"] != "svc" {
		t.Fatalf("unexpected get response: meta=%+v data=%#v", meta, data)
	}
	if _, _, err := store.Get("prod-vars", "wrong"); err == nil {
		t.Fatalf("expected wrong passphrase to fail")
	}

	rotation, err := store.Rotate("pass-v1", "pass-v2")
	if err != nil {
		t.Fatalf("rotate failed: %v", err)
	}
	if rotation.CurrentKeyVersion != 2 || rotation.RotatedFiles != 1 {
		t.Fatalf("unexpected rotation result: %+v", rotation)
	}

	if _, _, err := store.Get("prod-vars", "pass-v1"); err == nil {
		t.Fatalf("expected old passphrase to fail after rotation")
	}
	data, meta, err = store.Get("prod-vars", "pass-v2")
	if err != nil {
		t.Fatalf("get with new passphrase failed: %v", err)
	}
	if meta.KeyVersion != 2 || data["db_pass"] != "secret" {
		t.Fatalf("unexpected decrypted data after rotation: meta=%+v data=%#v", meta, data)
	}
}

func TestEncryptedVariableStorePersistsAcrossReload(t *testing.T) {
	baseDir := t.TempDir()
	store := NewEncryptedVariableStore(baseDir)
	_, err := store.Upsert("staging-vars", map[string]any{"feature_flag": true}, "rotate-me")
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	if _, err := store.Rotate("rotate-me", "rotate-new"); err != nil {
		t.Fatalf("rotate failed: %v", err)
	}

	reloaded := NewEncryptedVariableStore(baseDir)
	status := reloaded.KeyStatus()
	if status.CurrentKeyVersion != 2 {
		t.Fatalf("expected persisted key version 2, got %+v", status)
	}
	items := reloaded.List()
	if len(items) != 1 || items[0].Name != "staging-vars" {
		t.Fatalf("unexpected persisted files: %#v", items)
	}
	if _, _, err := reloaded.Get("staging-vars", "rotate-new"); err != nil {
		t.Fatalf("expected persisted encrypted file to decrypt: %v", err)
	}
}
