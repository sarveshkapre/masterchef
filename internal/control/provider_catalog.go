package control

import (
	"errors"
	"sort"
	"strings"
)

type ProviderProfile struct {
	ID                string   `json:"id"`
	Category          string   `json:"category"` // container|kubernetes|cloud
	Capabilities      []string `json:"capabilities"`
	SideEffects       []string `json:"side_effects,omitempty"`
	Purity            string   `json:"purity"` // pure|convergent|imperative
	PurityDescription string   `json:"purity_description,omitempty"`
}

type ProviderCatalogValidationInput struct {
	ProviderID           string   `json:"provider_id"`
	RequiredCapabilities []string `json:"required_capabilities"`
	RequiredPurity       string   `json:"required_purity,omitempty"`
	AllowedSideEffects   []string `json:"allowed_side_effects,omitempty"`
	DeniedSideEffects    []string `json:"denied_side_effects,omitempty"`
}

type ProviderCatalogValidation struct {
	Valid                 bool     `json:"valid"`
	ProviderID            string   `json:"provider_id"`
	Purity                string   `json:"purity,omitempty"`
	PuritySatisfied       bool     `json:"purity_satisfied"`
	SideEffects           []string `json:"side_effects,omitempty"`
	Missing               []string `json:"missing,omitempty"`
	DisallowedSideEffects []string `json:"disallowed_side_effects,omitempty"`
	Reason                string   `json:"reason"`
}

type ProviderCatalog struct {
	profiles map[string]ProviderProfile
}

func NewProviderCatalog() *ProviderCatalog {
	items := []ProviderProfile{
		{
			ID:                "container.docker",
			Category:          "container",
			Capabilities:      []string{"image_pull", "container_run", "container_stop", "artifact_mount"},
			SideEffects:       []string{"filesystem", "process", "network"},
			Purity:            "convergent",
			PurityDescription: "Convergent container lifecycle operations with bounded runtime and filesystem side effects.",
		},
		{
			ID:                "container.containerd",
			Category:          "container",
			Capabilities:      []string{"image_pull", "container_run", "container_stop"},
			SideEffects:       []string{"filesystem", "process", "network"},
			Purity:            "convergent",
			PurityDescription: "Convergent container lifecycle operations using the containerd runtime.",
		},
		{
			ID:                "kubernetes.core",
			Category:          "kubernetes",
			Capabilities:      []string{"apply_manifest", "rollout_status", "service_update", "configmap_sync", "secret_sync"},
			SideEffects:       []string{"api_server", "network"},
			Purity:            "convergent",
			PurityDescription: "Declarative API reconciliation with bounded control-plane side effects.",
		},
		{
			ID:                "kubernetes.helm",
			Category:          "kubernetes",
			Capabilities:      []string{"chart_install", "chart_upgrade", "chart_rollback"},
			SideEffects:       []string{"api_server", "network"},
			Purity:            "imperative",
			PurityDescription: "Release-oriented imperative actions that may trigger hook-managed side effects.",
		},
		{
			ID:                "cloud.aws",
			Category:          "cloud",
			Capabilities:      []string{"ec2_instance", "iam_role", "rds_instance", "s3_bucket", "vpc_network"},
			SideEffects:       []string{"cloud_api", "network", "identity"},
			Purity:            "imperative",
			PurityDescription: "Imperative cloud API operations with identity and network control-plane side effects.",
		},
		{
			ID:                "cloud.azure",
			Category:          "cloud",
			Capabilities:      []string{"vm_instance", "managed_identity", "sql_database", "storage_account", "virtual_network"},
			SideEffects:       []string{"cloud_api", "network", "identity"},
			Purity:            "imperative",
			PurityDescription: "Imperative cloud API operations across identity, network, and compute surfaces.",
		},
		{
			ID:                "cloud.gcp",
			Category:          "cloud",
			Capabilities:      []string{"compute_instance", "service_account", "cloud_sql", "gcs_bucket", "vpc_network"},
			SideEffects:       []string{"cloud_api", "network", "identity"},
			Purity:            "imperative",
			PurityDescription: "Imperative cloud API operations with externalized network and IAM side effects.",
		},
	}
	out := &ProviderCatalog{profiles: map[string]ProviderProfile{}}
	for _, item := range items {
		cp := item
		cp.Capabilities = normalizeStringList(cp.Capabilities)
		cp.SideEffects = normalizeStringList(cp.SideEffects)
		cp.Purity = normalizeProviderPurity(cp.Purity)
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
			Valid:                 false,
			ProviderID:            providerID,
			PuritySatisfied:       false,
			DisallowedSideEffects: nil,
			Reason:                "provider not found",
		}, nil
	}
	required := normalizeStringList(in.RequiredCapabilities)
	requiredPurity := normalizeProviderPurity(in.RequiredPurity)
	if strings.TrimSpace(in.RequiredPurity) != "" && requiredPurity == "" {
		return ProviderCatalogValidation{}, errors.New("required_purity must be one of: pure, convergent, imperative")
	}
	allowedSideEffects := normalizeStringList(in.AllowedSideEffects)
	deniedSideEffects := normalizeStringList(in.DeniedSideEffects)
	disallowed := validateProviderSideEffects(item.SideEffects, allowedSideEffects, deniedSideEffects)

	if len(required) == 0 {
		puritySatisfied := providerPuritySatisfies(item.Purity, requiredPurity)
		valid := puritySatisfied && len(disallowed) == 0
		reason := "provider exists and no required capabilities were specified"
		if !puritySatisfied {
			reason = "provider purity does not satisfy required_purity"
		} else if len(disallowed) > 0 {
			reason = "provider declares disallowed side effects"
		}
		return ProviderCatalogValidation{
			Valid:                 valid,
			ProviderID:            providerID,
			Purity:                item.Purity,
			PuritySatisfied:       puritySatisfied,
			SideEffects:           append([]string{}, item.SideEffects...),
			DisallowedSideEffects: disallowed,
			Reason:                reason,
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
			Valid:                 false,
			ProviderID:            providerID,
			Purity:                item.Purity,
			PuritySatisfied:       providerPuritySatisfies(item.Purity, requiredPurity),
			SideEffects:           append([]string{}, item.SideEffects...),
			Missing:               missing,
			DisallowedSideEffects: disallowed,
			Reason:                "provider missing required capabilities",
		}, nil
	}
	puritySatisfied := providerPuritySatisfies(item.Purity, requiredPurity)
	if !puritySatisfied {
		return ProviderCatalogValidation{
			Valid:                 false,
			ProviderID:            providerID,
			Purity:                item.Purity,
			PuritySatisfied:       false,
			SideEffects:           append([]string{}, item.SideEffects...),
			DisallowedSideEffects: disallowed,
			Reason:                "provider purity does not satisfy required_purity",
		}, nil
	}
	if len(disallowed) > 0 {
		return ProviderCatalogValidation{
			Valid:                 false,
			ProviderID:            providerID,
			Purity:                item.Purity,
			PuritySatisfied:       true,
			SideEffects:           append([]string{}, item.SideEffects...),
			DisallowedSideEffects: disallowed,
			Reason:                "provider declares disallowed side effects",
		}, nil
	}
	return ProviderCatalogValidation{
		Valid:                 true,
		ProviderID:            providerID,
		Purity:                item.Purity,
		PuritySatisfied:       true,
		SideEffects:           append([]string{}, item.SideEffects...),
		DisallowedSideEffects: nil,
		Reason:                "provider supports required capabilities, purity, and side-effect constraints",
	}, nil
}

func validateProviderSideEffects(declared, allowed, denied []string) []string {
	disallowed := make([]string, 0)
	allowSet := map[string]struct{}{}
	for _, item := range allowed {
		allowSet[item] = struct{}{}
	}
	denySet := map[string]struct{}{}
	for _, item := range denied {
		denySet[item] = struct{}{}
	}
	for _, effect := range declared {
		if len(allowSet) > 0 {
			if _, ok := allowSet[effect]; !ok {
				disallowed = append(disallowed, effect)
				continue
			}
		}
		if _, blocked := denySet[effect]; blocked {
			disallowed = append(disallowed, effect)
		}
	}
	return normalizeStringList(disallowed)
}

func normalizeProviderPurity(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pure", "convergent", "imperative":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func providerPuritySatisfies(actual, required string) bool {
	required = normalizeProviderPurity(required)
	if required == "" {
		return true
	}
	actual = normalizeProviderPurity(actual)
	if actual == "" {
		return false
	}
	rank := map[string]int{
		"imperative": 1,
		"convergent": 2,
		"pure":       3,
	}
	return rank[actual] >= rank[required]
}
