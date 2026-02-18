package control

import (
	"errors"
	"sort"
	"strings"
)

type SolutionPack struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Category          string   `json:"category"`
	Description       string   `json:"description"`
	RecommendedTags   []string `json:"recommended_tags,omitempty"`
	StarterConfigYAML string   `json:"starter_config_yaml"`
}

type SolutionPackCatalog struct {
	packs map[string]SolutionPack
}

func NewSolutionPackCatalog() *SolutionPackCatalog {
	packs := []SolutionPack{
		{
			ID:          "stateless-vm-service",
			Name:        "Stateless Service (VM)",
			Category:    "workspace-template",
			Description: "Starter for stateless service rollouts on VM fleets with safe rolling updates.",
			RecommendedTags: []string{
				"stateless", "vm", "rolling-update",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: app-01
      transport: local
resources:
  - id: deploy-artifact
    type: file
    host: app-01
    path: ./tmp/app-release.txt
    content: "service release artifact\n"
`,
		},
		{
			ID:          "stateful-postgres-cluster",
			Name:        "Stateful PostgreSQL Cluster",
			Category:    "solution-pack",
			Description: "Starter workflow for controlled stateful database maintenance and patching.",
			RecommendedTags: []string{
				"stateful", "postgres", "maintenance-window",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: db-01
      transport: local
resources:
  - id: postgres-config
    type: file
    host: db-01
    path: ./tmp/postgres.conf
    content: "max_connections=300\n"
`,
		},
		{
			ID:          "edge-disconnected-fleet",
			Name:        "Edge / Disconnected Fleet",
			Category:    "solution-pack",
			Description: "Starter for edge fleets with intermittent connectivity and local-first execution.",
			RecommendedTags: []string{
				"edge", "disconnected", "local-exec",
			},
			StarterConfigYAML: `version: v0
inventory:
  hosts:
    - name: edge-01
      transport: local
resources:
  - id: edge-runtime
    type: file
    host: edge-01
    path: ./tmp/edge-runtime.conf
    content: "mode=disconnected-first\n"
`,
		},
	}
	out := map[string]SolutionPack{}
	for _, p := range packs {
		out[p.ID] = p
	}
	return &SolutionPackCatalog{packs: out}
}

func (c *SolutionPackCatalog) List() []SolutionPack {
	out := make([]SolutionPack, 0, len(c.packs))
	for _, p := range c.packs {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (c *SolutionPackCatalog) Get(id string) (SolutionPack, error) {
	id = strings.TrimSpace(id)
	p, ok := c.packs[id]
	if !ok {
		return SolutionPack{}, errors.New("solution pack not found")
	}
	return p, nil
}
