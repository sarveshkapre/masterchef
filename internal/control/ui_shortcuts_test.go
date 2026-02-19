package control

import "testing"

func TestUIShortcutCatalogListAndSearch(t *testing.T) {
	catalog := NewUIShortcutCatalog()
	items := catalog.List()
	if len(items) < 5 {
		t.Fatalf("expected default shortcuts, got %d", len(items))
	}
	matches := catalog.Search("incident")
	if len(matches) == 0 {
		t.Fatalf("expected incident shortcut match")
	}
	found := false
	for _, item := range matches {
		if item.ID == "incident-view" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected incident-view shortcut in search results")
	}
}
