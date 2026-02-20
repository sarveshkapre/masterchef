package control

import "testing"

func TestProviderCatalogListAndValidate(t *testing.T) {
	catalog := NewProviderCatalog()
	items := catalog.List()
	if len(items) == 0 {
		t.Fatalf("expected provider profiles")
	}
	if items[0].Purity == "" {
		t.Fatalf("expected provider purity metadata")
	}
	valid, err := catalog.Validate(ProviderCatalogValidationInput{
		ProviderID:           "kubernetes.core",
		RequiredCapabilities: []string{"apply_manifest", "rollout_status"},
		RequiredPurity:       "convergent",
		AllowedSideEffects:   []string{"api_server", "network"},
	})
	if err != nil {
		t.Fatalf("validate provider failed: %v", err)
	}
	if !valid.Valid || !valid.PuritySatisfied {
		t.Fatalf("expected valid provider capabilities, got %+v", valid)
	}
	invalid, err := catalog.Validate(ProviderCatalogValidationInput{
		ProviderID:           "cloud.aws",
		RequiredCapabilities: []string{"chart_install"},
	})
	if err != nil {
		t.Fatalf("validate provider failed: %v", err)
	}
	if invalid.Valid || len(invalid.Missing) == 0 {
		t.Fatalf("expected missing capability result, got %+v", invalid)
	}

	purityInvalid, err := catalog.Validate(ProviderCatalogValidationInput{
		ProviderID:     "kubernetes.helm",
		RequiredPurity: "pure",
	})
	if err != nil {
		t.Fatalf("validate provider purity failed: %v", err)
	}
	if purityInvalid.Valid || purityInvalid.PuritySatisfied {
		t.Fatalf("expected purity validation failure, got %+v", purityInvalid)
	}

	sideEffectInvalid, err := catalog.Validate(ProviderCatalogValidationInput{
		ProviderID:           "cloud.gcp",
		DeniedSideEffects:    []string{"network"},
		RequiredCapabilities: []string{"compute_instance"},
	})
	if err != nil {
		t.Fatalf("validate provider side effects failed: %v", err)
	}
	if sideEffectInvalid.Valid || len(sideEffectInvalid.DisallowedSideEffects) == 0 {
		t.Fatalf("expected side-effect validation failure, got %+v", sideEffectInvalid)
	}
}
