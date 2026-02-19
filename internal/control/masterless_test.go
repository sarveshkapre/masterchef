package control

import "testing"

func TestMasterlessRender(t *testing.T) {
	store := NewMasterlessStore()
	if _, err := store.SetMode(MasterlessModeInput{
		Enabled:         true,
		StateRoot:       "/var/lib/masterchef",
		DefaultStrategy: "merge-last",
	}); err != nil {
		t.Fatalf("enable masterless mode failed: %v", err)
	}

	result, err := store.Render(MasterlessRenderInput{
		StateTemplate: `package={{pillar.packages.nginx}}
env={{var.environment}}`,
		Layers: []PillarLayer{
			{Name: "defaults", Data: map[string]any{"packages": map[string]any{"nginx": "nginx"}}},
		},
		Vars:    map[string]string{"environment": "prod"},
		Lookups: []string{"packages.nginx"},
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !result.Deterministic {
		t.Fatalf("expected deterministic render")
	}
	if result.Lookups["packages.nginx"] != "nginx" {
		t.Fatalf("expected lookup to resolve, got %+v", result.Lookups)
	}
	if result.RenderedState == "" || result.RenderedState == `package={{pillar.packages.nginx}}
env={{var.environment}}` {
		t.Fatalf("expected rendered template substitutions")
	}
}

func TestMasterlessRenderGuards(t *testing.T) {
	store := NewMasterlessStore()
	if _, err := store.Render(MasterlessRenderInput{StateTemplate: "x"}); err == nil {
		t.Fatalf("expected disabled mode guard")
	}
	if _, err := store.SetMode(MasterlessModeInput{Enabled: true}); err == nil {
		t.Fatalf("expected state_root requirement error")
	}
	if _, err := store.SetMode(MasterlessModeInput{Enabled: true, StateRoot: "/tmp/mc", DefaultStrategy: "invalid"}); err != nil {
		t.Fatalf("expected strategy fallback, got %v", err)
	}
	result, err := store.Render(MasterlessRenderInput{
		StateTemplate: "x={{pillar.missing}}",
		Layers:        []PillarLayer{{Name: "base", Data: map[string]any{"foo": "bar"}}},
	})
	if err != nil {
		t.Fatalf("render with missing token should still return result: %v", err)
	}
	if result.Deterministic || len(result.MissingTokens) == 0 {
		t.Fatalf("expected missing token report: %+v", result)
	}
}
