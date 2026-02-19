package control

import "testing"

func TestAccessibilityStoreProfiles(t *testing.T) {
	store := NewAccessibilityStore()
	items := store.ListProfiles()
	if len(items) < 3 {
		t.Fatalf("expected built-in accessibility profiles, got %d", len(items))
	}
	active := store.ActiveProfile()
	if active.ID != "default" {
		t.Fatalf("expected default profile active, got %+v", active)
	}
}

func TestAccessibilityStoreUpsertAndActivate(t *testing.T) {
	store := NewAccessibilityStore()
	profile, err := store.UpsertProfile(AccessibilityProfile{
		ID:                    "sr-heavy",
		Name:                  "Screen Reader Heavy",
		KeyboardOnly:          true,
		ScreenReaderOptimized: true,
	})
	if err != nil {
		t.Fatalf("upsert profile failed: %v", err)
	}
	if profile.ID != "sr-heavy" {
		t.Fatalf("unexpected profile id: %+v", profile)
	}
	if _, err := store.SetActive("sr-heavy"); err != nil {
		t.Fatalf("set active failed: %v", err)
	}
	active := store.ActiveProfile()
	if active.ID != "sr-heavy" || !active.ScreenReaderOptimized {
		t.Fatalf("unexpected active profile: %+v", active)
	}
	if _, err := store.UpsertProfile(AccessibilityProfile{ID: "empty"}); err == nil {
		t.Fatalf("expected validation error for profile without accessibility modes")
	}
}
