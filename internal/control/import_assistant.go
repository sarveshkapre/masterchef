package control

import (
	"errors"
	"strings"
	"time"
)

type ImportAssistantRequest struct {
	Type         string   `json:"type"` // secrets|facts|role_hierarchy
	SourceSystem string   `json:"source_system,omitempty"`
	SampleFields []string `json:"sample_fields,omitempty"`
}

type ImportAssistantResult struct {
	Type             string            `json:"type"`
	SourceSystem     string            `json:"source_system,omitempty"`
	RequiredFields   []string          `json:"required_fields"`
	SuggestedMapping map[string]string `json:"suggested_mapping"`
	ValidationRules  []string          `json:"validation_rules"`
	NextSteps        []string          `json:"next_steps"`
	GeneratedAt      time.Time         `json:"generated_at"`
}

func BuildImportAssistant(req ImportAssistantRequest) (ImportAssistantResult, error) {
	typ := strings.ToLower(strings.TrimSpace(req.Type))
	source := strings.TrimSpace(req.SourceSystem)
	sample := normalizeStringSlice(req.SampleFields)
	if typ == "" {
		return ImportAssistantResult{}, errors.New("type is required")
	}
	res := ImportAssistantResult{
		Type:             typ,
		SourceSystem:     source,
		SuggestedMapping: map[string]string{},
		GeneratedAt:      time.Now().UTC(),
	}

	switch typ {
	case "secrets":
		res.RequiredFields = []string{"path", "value", "engine"}
		res.ValidationRules = []string{
			"secret paths must be unique",
			"secret values must be encrypted in transit",
			"do not include plaintext in logs",
		}
		res.NextSteps = []string{
			"import into secrets integrations via /v1/secrets/integrations",
			"validate retrieval with /v1/secrets/resolve",
		}
	case "facts":
		res.RequiredFields = []string{"node", "fact_key", "fact_value"}
		res.ValidationRules = []string{
			"fact keys must be normalized lowercase",
			"facts older than ttl should be refreshed",
			"high-cardinality facts should be scoped",
		}
		res.NextSteps = []string{
			"upsert facts via /v1/facts/cache",
			"validate queries with /v1/facts/mine/query",
		}
	case "role_hierarchy":
		res.RequiredFields = []string{"role", "environment", "default_attributes", "override_attributes"}
		res.ValidationRules = []string{
			"role names must be unique",
			"attribute precedence must be deterministic",
			"environment overrides should be explicit",
		}
		res.NextSteps = []string{
			"import roles via /v1/roles",
			"import environments via /v1/environments",
			"verify merged view via /v1/vars/resolve",
		}
	default:
		return ImportAssistantResult{}, errors.New("type must be secrets, facts, or role_hierarchy")
	}

	for _, field := range sample {
		key := strings.ToLower(strings.TrimSpace(field))
		if key == "" {
			continue
		}
		mapped := key
		switch typ {
		case "secrets":
			if strings.Contains(key, "secret") {
				mapped = "value"
			}
			if strings.Contains(key, "name") {
				mapped = "path"
			}
		case "facts":
			if strings.Contains(key, "host") || strings.Contains(key, "node") {
				mapped = "node"
			}
			if strings.Contains(key, "key") {
				mapped = "fact_key"
			}
		case "role_hierarchy":
			if strings.Contains(key, "group") {
				mapped = "role"
			}
			if strings.Contains(key, "env") {
				mapped = "environment"
			}
		}
		res.SuggestedMapping[key] = mapped
	}
	return res, nil
}
