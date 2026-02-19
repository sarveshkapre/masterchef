package control

import "testing"

func TestProviderSandboxEvaluate(t *testing.T) {
	store := NewProviderSandboxStore()
	if _, err := store.UpsertProfile(ProviderSandboxProfileInput{
		Provider:        "provider.custom",
		Runtime:         "wasi",
		Capabilities:    []string{"exec", "read_state"},
		FilesystemScope: []string{"/var/lib/masterchef/providers/custom"},
		NetworkScope:    []string{"api.internal:443"},
	}); err != nil {
		t.Fatalf("upsert provider sandbox profile failed: %v", err)
	}

	eval := store.Evaluate(ProviderSandboxEvaluateInput{
		Provider:             "provider.custom",
		Untrusted:            true,
		RequiredCapabilities: []string{"exec"},
		RequireFilesystem:    true,
		RequireNetwork:       true,
	})
	if !eval.Allowed {
		t.Fatalf("expected evaluation to allow profile, got %+v", eval)
	}
}

func TestProviderSandboxRejectsUnsafeProfile(t *testing.T) {
	store := NewProviderSandboxStore()
	if _, err := store.UpsertProfile(ProviderSandboxProfileInput{
		Provider:        "provider.unsafe",
		Runtime:         "native",
		Capabilities:    []string{"read_state"},
		AllowHostAccess: true,
	}); err != nil {
		t.Fatalf("upsert provider sandbox profile failed: %v", err)
	}
	eval := store.Evaluate(ProviderSandboxEvaluateInput{
		Provider:             "provider.unsafe",
		Untrusted:            true,
		RequiredCapabilities: []string{"exec"},
		RequireFilesystem:    true,
	})
	if eval.Allowed {
		t.Fatalf("expected unsafe profile deny, got %+v", eval)
	}
	if len(eval.Violations) == 0 || len(eval.MissingCapabilities) == 0 {
		t.Fatalf("expected violations and missing capabilities, got %+v", eval)
	}
}
