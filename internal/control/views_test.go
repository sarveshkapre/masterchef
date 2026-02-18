package control

import "testing"

func TestSavedViewStoreLifecycle(t *testing.T) {
	store := NewSavedViewStore()
	view, err := store.Create(SavedView{
		Name:   "prod alerts",
		Entity: "alerts",
		Mode:   "human",
		Query:  "severity=high",
		Limit:  50,
	})
	if err != nil {
		t.Fatalf("create saved view failed: %v", err)
	}
	if view.ID == "" || view.ShareToken == "" {
		t.Fatalf("expected id and share token")
	}

	view, err = store.SetPinned(view.ID, true)
	if err != nil {
		t.Fatalf("pin saved view failed: %v", err)
	}
	if !view.Pinned {
		t.Fatalf("expected pinned view")
	}

	updated, err := store.RegenerateShareToken(view.ID)
	if err != nil {
		t.Fatalf("regenerate share token failed: %v", err)
	}
	if updated.ShareToken == view.ShareToken {
		t.Fatalf("expected new share token")
	}

	if err := store.Delete(view.ID); err != nil {
		t.Fatalf("delete saved view failed: %v", err)
	}
}
