package control

import "testing"

func TestCompatibilityShimStoreBuiltinsAndResolve(t *testing.T) {
	store := NewCompatibilityShimStore()
	items := store.List()
	if len(items) < 4 {
		t.Fatalf("expected built-in compatibility shims, got %d", len(items))
	}
	result := store.Resolve(CompatibilityShimResolveInput{
		SourcePlatform: "ansible",
		Content:        "- name: x\n  with_items: [a, b]\n",
	})
	if len(result.Matched) == 0 {
		t.Fatalf("expected compatibility shim resolve match, got %+v", result)
	}
	if result.CoverageScore <= 0 {
		t.Fatalf("expected positive coverage score, got %+v", result)
	}
}

func TestCompatibilityShimStoreUpsertAndToggle(t *testing.T) {
	store := NewCompatibilityShimStore()
	item, err := store.Upsert(CompatibilityShimInput{
		SourcePlatform:  "chef",
		LegacyPattern:   "legacy execute shell wrappers",
		Description:     "Map shell wrappers to typed command resources with guards.",
		Target:          "command resource",
		Keywords:        []string{"execute", "shell_out"},
		RiskLevel:       "medium",
		Recommendation:  "replace shell wrappers with command resources and only_if guards",
		ConvergenceSafe: true,
	})
	if err != nil {
		t.Fatalf("upsert shim failed: %v", err)
	}
	if item.ID == "" || !item.Enabled {
		t.Fatalf("unexpected upserted shim: %+v", item)
	}

	disabled, err := store.Disable(item.ID)
	if err != nil {
		t.Fatalf("disable shim failed: %v", err)
	}
	if disabled.Enabled {
		t.Fatalf("expected shim to be disabled")
	}

	enabled, err := store.Enable(item.ID)
	if err != nil {
		t.Fatalf("enable shim failed: %v", err)
	}
	if !enabled.Enabled {
		t.Fatalf("expected shim to be enabled")
	}
}
