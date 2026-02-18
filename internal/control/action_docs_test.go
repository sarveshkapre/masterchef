package control

import "testing"

func TestActionDocCatalog(t *testing.T) {
	c := NewActionDocCatalog()
	list := c.List()
	if len(list) < 4 {
		t.Fatalf("expected built-in action docs, got %d", len(list))
	}
	item, err := c.Get("investigate-failed-run")
	if err != nil {
		t.Fatalf("get action doc failed: %v", err)
	}
	if len(item.Endpoints) == 0 {
		t.Fatalf("expected action doc endpoints")
	}
}
