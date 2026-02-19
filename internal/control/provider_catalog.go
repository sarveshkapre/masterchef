package control

import (
	"errors"
	"sort"
	"strings"
)

type ProviderProfile struct {
	ID           string   `json:"id"`
	Category     string   `json:"category"` // container|kubernetes|cloud
	Capabilities []string `json:"capabilities"`
}

type ProviderCatalogValidationInput struct {
	ProviderID           string   `json:"provider_id"`
	RequiredCapabilities []string `json:"required_capabilities"`
}

type ProviderCatalogValidation struct {
	Valid      bool     `json:"valid"`
	ProviderID string   `json:"provider_id"`
	Missing    []string `json:"missing,omitempty"`
	Reason     string   `json:"reason"`
}

type ProviderCatalog struct {
	profiles map[string]ProviderProfile
}

func NewProviderCatalog() *ProviderCatalog {
	items := []ProviderProfile{
		{ID: "container.docker", Category: "container", Capabilities: []string{"image_pull", "container_run", "container_stop", "artifact_mount"}},
		{ID: "container.containerd", Category: "container", Capabilities: []string{"image_pull", "container_run", "container_stop"}},
		{ID: "kubernetes.core", Category: "kubernetes", Capabilities: []string{"apply_manifest", "rollout_status", "service_update", "configmap_sync", "secret_sync"}},
		{ID: "kubernetes.helm", Category: "kubernetes", Capabilities: []string{"chart_install", "chart_upgrade", "chart_rollback"}},
		{ID: "cloud.aws", Category: "cloud", Capabilities: []string{"ec2_instance", "iam_role", "rds_instance", "s3_bucket", "vpc_network"}},
		{ID: "cloud.azure", Category: "cloud", Capabilities: []string{"vm_instance", "managed_identity", "sql_database", "storage_account", "virtual_network"}},
		{ID: "cloud.gcp", Category: "cloud", Capabilities: []string{"compute_instance", "service_account", "cloud_sql", "gcs_bucket", "vpc_network"}},
	}
	out := &ProviderCatalog{profiles: map[string]ProviderProfile{}}
	for _, item := range items {
		cp := item
		cp.Capabilities = normalizeStringList(cp.Capabilities)
		out.profiles[cp.ID] = cp
	}
	return out
}

func (c *ProviderCatalog) List() []ProviderProfile {
	out := make([]ProviderProfile, 0, len(c.profiles))
	for _, item := range c.profiles {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (c *ProviderCatalog) Get(id string) (ProviderProfile, bool) {
	item, ok := c.profiles[strings.ToLower(strings.TrimSpace(id))]
	if !ok {
		return ProviderProfile{}, false
	}
	return item, true
}

func (c *ProviderCatalog) Validate(in ProviderCatalogValidationInput) (ProviderCatalogValidation, error) {
	providerID := strings.ToLower(strings.TrimSpace(in.ProviderID))
	if providerID == "" {
		return ProviderCatalogValidation{}, errors.New("provider_id is required")
	}
	item, ok := c.profiles[providerID]
	if !ok {
		return ProviderCatalogValidation{
			Valid:      false,
			ProviderID: providerID,
			Reason:     "provider not found",
		}, nil
	}
	required := normalizeStringList(in.RequiredCapabilities)
	if len(required) == 0 {
		return ProviderCatalogValidation{
			Valid:      true,
			ProviderID: providerID,
			Reason:     "provider exists and no required capabilities were specified",
		}, nil
	}
	capSet := map[string]struct{}{}
	for _, capability := range item.Capabilities {
		capSet[capability] = struct{}{}
	}
	missing := make([]string, 0)
	for _, capability := range required {
		if _, ok := capSet[capability]; !ok {
			missing = append(missing, capability)
		}
	}
	if len(missing) > 0 {
		return ProviderCatalogValidation{
			Valid:      false,
			ProviderID: providerID,
			Missing:    missing,
			Reason:     "provider missing required capabilities",
		}, nil
	}
	return ProviderCatalogValidation{
		Valid:      true,
		ProviderID: providerID,
		Reason:     "provider supports all required capabilities",
	}, nil
}
