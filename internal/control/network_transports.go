package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type NetworkTransportInput struct {
	Name                     string            `json:"name"`
	Description              string            `json:"description,omitempty"`
	Category                 string            `json:"category,omitempty"`
	DefaultPort              int               `json:"default_port,omitempty"`
	SupportsConfigPush       bool              `json:"supports_config_push"`
	SupportsTelemetryPull    bool              `json:"supports_telemetry_pull"`
	CredentialFieldsRequired []string          `json:"credential_fields_required,omitempty"`
	Metadata                 map[string]string `json:"metadata,omitempty"`
}

type NetworkTransport struct {
	Name                     string            `json:"name"`
	Description              string            `json:"description,omitempty"`
	Category                 string            `json:"category"`
	DefaultPort              int               `json:"default_port,omitempty"`
	Builtin                  bool              `json:"builtin"`
	SupportsConfigPush       bool              `json:"supports_config_push"`
	SupportsTelemetryPull    bool              `json:"supports_telemetry_pull"`
	CredentialFieldsRequired []string          `json:"credential_fields_required,omitempty"`
	Metadata                 map[string]string `json:"metadata,omitempty"`
	UpdatedAt                time.Time         `json:"updated_at"`
}

type NetworkTransportValidation struct {
	Input     string `json:"input"`
	Canonical string `json:"canonical"`
	Supported bool   `json:"supported"`
	Builtin   bool   `json:"builtin"`
	Category  string `json:"category,omitempty"`
	Message   string `json:"message,omitempty"`
}

type NetworkTransportCatalog struct {
	mu    sync.RWMutex
	items map[string]NetworkTransport
}

func NewNetworkTransportCatalog() *NetworkTransportCatalog {
	now := time.Now().UTC()
	items := map[string]NetworkTransport{}
	items["netconf"] = NetworkTransport{
		Name:                  "netconf",
		Description:           "NETCONF over SSH for transactional network device configuration",
		Category:              "netconf",
		DefaultPort:           830,
		Builtin:               true,
		SupportsConfigPush:    true,
		SupportsTelemetryPull: true,
		CredentialFieldsRequired: []string{
			"username",
			"password_or_private_key",
		},
		UpdatedAt: now,
	}
	items["restconf"] = NetworkTransport{
		Name:                  "restconf",
		Description:           "RESTCONF API-driven network device management",
		Category:              "restconf",
		DefaultPort:           443,
		Builtin:               true,
		SupportsConfigPush:    true,
		SupportsTelemetryPull: true,
		CredentialFieldsRequired: []string{
			"bearer_token_or_basic_auth",
		},
		UpdatedAt: now,
	}
	items["api"] = NetworkTransport{
		Name:                  "api",
		Description:           "Generic API-driven device integration",
		Category:              "api",
		DefaultPort:           443,
		Builtin:               true,
		SupportsConfigPush:    true,
		SupportsTelemetryPull: true,
		CredentialFieldsRequired: []string{
			"endpoint",
			"api_token",
		},
		UpdatedAt: now,
	}
	return &NetworkTransportCatalog{items: items}
}

func (c *NetworkTransportCatalog) Register(in NetworkTransportInput) (NetworkTransport, error) {
	name := strings.ToLower(strings.TrimSpace(in.Name))
	if name == "" {
		return NetworkTransport{}, errors.New("name is required")
	}
	if name == "plugin" || strings.HasPrefix(name, "plugin/") {
		return NetworkTransport{}, errors.New("custom transport names cannot use reserved plugin prefix")
	}
	category := normalizeNetworkTransportCategory(in.Category)
	item := NetworkTransport{
		Name:                     name,
		Description:              strings.TrimSpace(in.Description),
		Category:                 category,
		DefaultPort:              in.DefaultPort,
		Builtin:                  false,
		SupportsConfigPush:       in.SupportsConfigPush,
		SupportsTelemetryPull:    in.SupportsTelemetryPull,
		CredentialFieldsRequired: normalizeStringSlice(in.CredentialFieldsRequired),
		Metadata:                 normalizeNetworkTransportMetadata(in.Metadata),
		UpdatedAt:                time.Now().UTC(),
	}
	c.mu.Lock()
	c.items[name] = item
	c.mu.Unlock()
	return cloneNetworkTransport(item), nil
}

func (c *NetworkTransportCatalog) List() []NetworkTransport {
	c.mu.RLock()
	out := make([]NetworkTransport, 0, len(c.items))
	for _, item := range c.items {
		out = append(out, cloneNetworkTransport(item))
	}
	c.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (c *NetworkTransportCatalog) Get(name string) (NetworkTransport, bool) {
	canonical := strings.ToLower(strings.TrimSpace(name))
	c.mu.RLock()
	item, ok := c.items[canonical]
	c.mu.RUnlock()
	if !ok {
		return NetworkTransport{}, false
	}
	return cloneNetworkTransport(item), true
}

func (c *NetworkTransportCatalog) Validate(name string) (NetworkTransportValidation, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return NetworkTransportValidation{}, errors.New("transport is required")
	}
	canonical := strings.ToLower(trimmed)
	if strings.HasPrefix(canonical, "plugin/") {
		return NetworkTransportValidation{
			Input:     trimmed,
			Canonical: canonical,
			Supported: true,
			Builtin:   false,
			Category:  "plugin",
			Message:   "plugin transport accepted",
		}, nil
	}
	if item, ok := c.Get(canonical); ok {
		return NetworkTransportValidation{
			Input:     trimmed,
			Canonical: item.Name,
			Supported: true,
			Builtin:   item.Builtin,
			Category:  item.Category,
			Message:   "transport supported",
		}, nil
	}
	return NetworkTransportValidation{
		Input:     trimmed,
		Canonical: canonical,
		Supported: false,
		Message:   "unsupported transport; use netconf, restconf, api, plugin/<name>, or register a custom transport",
	}, nil
}

func normalizeNetworkTransportCategory(in string) string {
	value := strings.ToLower(strings.TrimSpace(in))
	switch value {
	case "netconf", "restconf", "api", "plugin", "custom":
		return value
	default:
		return "custom"
	}
}

func cloneNetworkTransport(in NetworkTransport) NetworkTransport {
	out := in
	out.CredentialFieldsRequired = append([]string{}, in.CredentialFieldsRequired...)
	out.Metadata = normalizeNetworkTransportMetadata(in.Metadata)
	return out
}

func normalizeNetworkTransportMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(v)
	}
	return out
}
