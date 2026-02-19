package control

import (
	"errors"
	"strings"
	"time"
)

type BrownfieldObservedHost struct {
	Name      string   `json:"name"`
	Address   string   `json:"address,omitempty"`
	Transport string   `json:"transport,omitempty"`
	Packages  []string `json:"packages,omitempty"`
	Services  []string `json:"services,omitempty"`
	Files     []string `json:"files,omitempty"`
}

type BrownfieldBootstrapRequest struct {
	Hosts []BrownfieldObservedHost `json:"hosts"`
}

type BrownfieldBaselineResource struct {
	ID   string         `json:"id"`
	Type string         `json:"type"`
	Host string         `json:"host"`
	Spec map[string]any `json:"spec"`
}

type BrownfieldBootstrapResult struct {
	GeneratedAt    time.Time                    `json:"generated_at"`
	InventoryHosts []NodeEnrollInput            `json:"inventory_hosts"`
	Resources      []BrownfieldBaselineResource `json:"resources"`
	Counts         map[string]int               `json:"counts"`
}

func BuildBrownfieldBaseline(req BrownfieldBootstrapRequest) (BrownfieldBootstrapResult, error) {
	if len(req.Hosts) == 0 {
		return BrownfieldBootstrapResult{}, errors.New("hosts are required")
	}
	result := BrownfieldBootstrapResult{
		GeneratedAt:    time.Now().UTC(),
		InventoryHosts: make([]NodeEnrollInput, 0, len(req.Hosts)),
		Resources:      []BrownfieldBaselineResource{},
		Counts:         map[string]int{},
	}
	for _, host := range req.Hosts {
		name := strings.TrimSpace(host.Name)
		if name == "" {
			return BrownfieldBootstrapResult{}, errors.New("host name is required")
		}
		transport := strings.ToLower(strings.TrimSpace(host.Transport))
		if transport == "" {
			transport = "ssh"
		}
		result.InventoryHosts = append(result.InventoryHosts, NodeEnrollInput{
			Name:      name,
			Address:   strings.TrimSpace(host.Address),
			Transport: transport,
			Source:    "brownfield-bootstrap",
		})

		for _, pkg := range normalizeStringSlice(host.Packages) {
			result.Resources = append(result.Resources, BrownfieldBaselineResource{
				ID:   "brownfield-" + name + "-pkg-" + sanitizeResourcePart(pkg),
				Type: "package",
				Host: name,
				Spec: map[string]any{"name": pkg, "state": "present"},
			})
			result.Counts["package"]++
		}
		for _, svc := range normalizeStringSlice(host.Services) {
			result.Resources = append(result.Resources, BrownfieldBaselineResource{
				ID:   "brownfield-" + name + "-svc-" + sanitizeResourcePart(svc),
				Type: "service",
				Host: name,
				Spec: map[string]any{"name": svc, "state": "running", "enabled": true},
			})
			result.Counts["service"]++
		}
		for _, file := range normalizeStringSlice(host.Files) {
			result.Resources = append(result.Resources, BrownfieldBaselineResource{
				ID:   "brownfield-" + name + "-file-" + sanitizeResourcePart(file),
				Type: "file",
				Host: name,
				Spec: map[string]any{"path": file, "mode": "preserve"},
			})
			result.Counts["file"]++
		}
	}
	result.Counts["hosts"] = len(result.InventoryHosts)
	result.Counts["resources"] = len(result.Resources)
	return result, nil
}

func sanitizeResourcePart(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	replacer := strings.NewReplacer("/", "-", ".", "-", " ", "-", "_", "-")
	raw = replacer.Replace(raw)
	raw = strings.Trim(raw, "-")
	if raw == "" {
		return "item"
	}
	return raw
}
