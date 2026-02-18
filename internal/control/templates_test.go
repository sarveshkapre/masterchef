package control

import "testing"

func TestTemplateStore_CreateGetDelete(t *testing.T) {
	s := NewTemplateStore()
	tpl := s.Create(Template{
		Name:       "demo",
		ConfigPath: "x.yaml",
	})
	if tpl.ID == "" {
		t.Fatalf("expected template id")
	}
	got, ok := s.Get(tpl.ID)
	if !ok || got.Name != "demo" {
		t.Fatalf("unexpected get result: %+v", got)
	}
	if err := s.Delete(tpl.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if _, ok := s.Get(tpl.ID); ok {
		t.Fatalf("expected deleted template to be gone")
	}
}
