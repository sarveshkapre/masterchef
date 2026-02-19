package control

import "testing"

func TestDashboardWidgetStoreLifecycle(t *testing.T) {
	store := NewDashboardWidgetStore()
	item, err := store.Create(DashboardWidget{
		ViewID: "view-1",
		Title:  "Fleet Health",
		Pinned: true,
		Width:  8,
		Height: 5,
		Column: 1,
		Row:    2,
	})
	if err != nil {
		t.Fatalf("create widget failed: %v", err)
	}
	if item.ID == "" || item.ViewID != "view-1" {
		t.Fatalf("unexpected widget: %+v", item)
	}

	items := store.List()
	if len(items) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(items))
	}
	if _, err := store.Get(item.ID); err != nil {
		t.Fatalf("get widget failed: %v", err)
	}

	refreshed, err := store.Refresh(item.ID)
	if err != nil {
		t.Fatalf("refresh widget failed: %v", err)
	}
	if refreshed.LastRefreshedAt.IsZero() {
		t.Fatalf("expected last_refreshed_at to be set")
	}

	unpinned, err := store.SetPinned(item.ID, false)
	if err != nil {
		t.Fatalf("set pinned failed: %v", err)
	}
	if unpinned.Pinned {
		t.Fatalf("expected widget to be unpinned")
	}

	if err := store.Delete(item.ID); err != nil {
		t.Fatalf("delete widget failed: %v", err)
	}
	if _, err := store.Get(item.ID); err == nil {
		t.Fatalf("expected deleted widget lookup to fail")
	}
}
