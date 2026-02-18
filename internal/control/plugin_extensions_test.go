package control

import "testing"

func TestPluginExtensionStoreCRUD(t *testing.T) {
	store := NewPluginExtensionStore()

	created, err := store.Create(PluginExtension{
		Name:       "slack-callback",
		Type:       PluginCallback,
		Entrypoint: "/plugins/slack/callback.so",
		Enabled:    true,
		Config: map[string]any{
			"channel": "#ops",
		},
	})
	if err != nil {
		t.Fatalf("create plugin failed: %v", err)
	}
	if created.ID == "" || created.Type != PluginCallback {
		t.Fatalf("unexpected plugin record: %+v", created)
	}

	items := store.List("")
	if len(items) != 1 {
		t.Fatalf("expected one plugin, got %#v", items)
	}
	filtered := store.List("lookup")
	if len(filtered) != 0 {
		t.Fatalf("expected no lookup plugins, got %#v", filtered)
	}

	got, err := store.Get(created.ID)
	if err != nil {
		t.Fatalf("get plugin failed: %v", err)
	}
	if got.Config["channel"] != "#ops" {
		t.Fatalf("unexpected plugin config: %#v", got.Config)
	}

	disabled, err := store.SetEnabled(created.ID, false)
	if err != nil {
		t.Fatalf("disable plugin failed: %v", err)
	}
	if disabled.Enabled {
		t.Fatalf("expected plugin to be disabled")
	}
	if !store.Delete(created.ID) {
		t.Fatalf("expected delete to succeed")
	}
	if store.Delete(created.ID) {
		t.Fatalf("expected second delete to fail")
	}
}

func TestPluginExtensionStoreValidation(t *testing.T) {
	store := NewPluginExtensionStore()
	if _, err := store.Create(PluginExtension{Name: "x", Type: "unknown", Entrypoint: "/tmp/x"}); err == nil {
		t.Fatalf("expected invalid type error")
	}
	if _, err := store.Create(PluginExtension{Name: "", Type: PluginLookup, Entrypoint: "/tmp/x"}); err == nil {
		t.Fatalf("expected missing name error")
	}
	if _, err := store.Create(PluginExtension{Name: "x", Type: PluginLookup, Entrypoint: ""}); err == nil {
		t.Fatalf("expected missing entrypoint error")
	}
}
