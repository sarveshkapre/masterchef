package control

import "testing"

func TestProviderCatalogListAndValidate(t *testing.T) {
	catalog := NewProviderCatalog()
	items := catalog.List()
	if len(items) == 0 {
		t.Fatalf("expected provider profiles")
	}
	valid, err := catalog.Validate(ProviderCatalogValidationInput{
		ProviderID:           "kubernetes.core",
		RequiredCapabilities: []string{"apply_manifest", "rollout_status"},
	})
	if err != nil {
		t.Fatalf("validate provider failed: %v", err)
	}
	if !valid.Valid {
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
}
