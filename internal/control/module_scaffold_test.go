package control

import "testing"

func TestModuleScaffoldCatalogListAndGet(t *testing.T) {
	catalog := NewModuleScaffoldCatalog()
	all := catalog.List("")
	if len(all) < 2 {
		t.Fatalf("expected module scaffold templates, got %d", len(all))
	}
	modules := catalog.List("module")
	if len(modules) != 1 || modules[0].Kind != "module" {
		t.Fatalf("expected module-only scaffold template list, got %+v", modules)
	}
	item, err := catalog.Get("provider-best-practice")
	if err != nil {
		t.Fatalf("get scaffold template failed: %v", err)
	}
	if item.Kind != "provider" || len(item.ScaffoldFiles) == 0 {
		t.Fatalf("unexpected provider scaffold template: %+v", item)
	}
}
