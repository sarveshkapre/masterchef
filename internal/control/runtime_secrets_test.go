package control

import (
	"testing"
	"time"
)

func TestRuntimeSecretMaterializeConsumeDestroy(t *testing.T) {
	store := NewRuntimeSecretStore()
	session, err := store.Materialize(RuntimeSecretSessionInput{
		Source:     "encrypted-vars:prod",
		TTLSeconds: 120,
		Data: map[string]any{
			"db_user": "svc",
			"db_pass": "top-secret",
		},
	})
	if err != nil {
		t.Fatalf("materialize runtime secret failed: %v", err)
	}
	if session.ID == "" {
		t.Fatalf("expected session id")
	}

	data, consumed, err := store.consumeAt(session.ID, session.CreatedAt.Add(10*time.Second))
	if err != nil {
		t.Fatalf("consume runtime secret failed: %v", err)
	}
	if data["db_user"] != "svc" || !consumed.Consumed {
		t.Fatalf("unexpected consumed payload/session: data=%#v session=%+v", data, consumed)
	}

	if _, _, err := store.Consume(session.ID); err == nil {
		t.Fatalf("expected second consume to fail")
	}

	other, err := store.Materialize(RuntimeSecretSessionInput{
		Source:     "encrypted-vars:staging",
		TTLSeconds: 120,
		Data:       map[string]any{"token": "abc"},
	})
	if err != nil {
		t.Fatalf("materialize second secret failed: %v", err)
	}
	destroyed, err := store.Destroy(other.ID)
	if err != nil {
		t.Fatalf("destroy session failed: %v", err)
	}
	if !destroyed.Destroyed {
		t.Fatalf("expected destroyed session state")
	}
}

func TestRuntimeSecretExpiry(t *testing.T) {
	store := NewRuntimeSecretStore()
	session, err := store.Materialize(RuntimeSecretSessionInput{
		Source:     "encrypted-vars:prod",
		TTLSeconds: 30,
		Data:       map[string]any{"key": "value"},
	})
	if err != nil {
		t.Fatalf("materialize runtime secret failed: %v", err)
	}
	if _, _, err := store.consumeAt(session.ID, session.ExpiresAt.Add(time.Second)); err == nil {
		t.Fatalf("expected expired session consume to fail")
	}
}
