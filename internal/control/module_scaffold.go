package control

import (
	"errors"
	"sort"
	"strings"
)

type ModuleScaffoldTemplate struct {
	ID            string            `json:"id"`
	Kind          string            `json:"kind"` // module|provider
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	BestPractices []string          `json:"best_practices,omitempty"`
	ScaffoldFiles map[string]string `json:"scaffold_files,omitempty"`
}

type ModuleScaffoldCatalog struct {
	templates map[string]ModuleScaffoldTemplate
}

func NewModuleScaffoldCatalog() *ModuleScaffoldCatalog {
	items := []ModuleScaffoldTemplate{
		{
			ID:          "module-best-practice",
			Kind:        "module",
			Name:        "Module Best-Practice Template",
			Description: "Scaffold for production-grade reusable modules.",
			BestPractices: []string{
				"Declare explicit input/output contracts",
				"Add deterministic unit tests and idempotency checks",
				"Document upgrade notes and compatibility guarantees",
			},
			ScaffoldFiles: map[string]string{
				"README.md": `# Module Template

Production-grade reusable module scaffold.
`,
				"module.yaml": `name: example/module
version: 0.1.0
kind: module
`,
				"tests/module_test.yaml": `suite: module
cases:
  - name: converges idempotently
`,
			},
		},
		{
			ID:          "provider-best-practice",
			Kind:        "provider",
			Name:        "Provider Best-Practice Template",
			Description: "Scaffold for provider plugins with conformance testing.",
			BestPractices: []string{
				"Declare capability metadata and side-effect boundaries",
				"Implement conformance and property-based tests",
				"Pin protocol version and backward compatibility contract",
			},
			ScaffoldFiles: map[string]string{
				"README.md": `# Provider Template

Production-grade provider scaffold.
`,
				"provider.yaml": `name: example/provider
version: 0.1.0
kind: provider
protocol: v1
`,
				"tests/provider_conformance_test.yaml": `suite: provider
cases:
  - name: idempotent action apply
`,
			},
		},
	}
	out := map[string]ModuleScaffoldTemplate{}
	for _, item := range items {
		out[item.ID] = item
	}
	return &ModuleScaffoldCatalog{templates: out}
}

func (c *ModuleScaffoldCatalog) List(kind string) []ModuleScaffoldTemplate {
	kind = strings.ToLower(strings.TrimSpace(kind))
	out := make([]ModuleScaffoldTemplate, 0, len(c.templates))
	for _, item := range c.templates {
		if kind != "" && item.Kind != kind {
			continue
		}
		out = append(out, cloneModuleScaffold(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (c *ModuleScaffoldCatalog) Get(id string) (ModuleScaffoldTemplate, error) {
	id = strings.TrimSpace(id)
	item, ok := c.templates[id]
	if !ok {
		return ModuleScaffoldTemplate{}, errors.New("module scaffold template not found")
	}
	return cloneModuleScaffold(item), nil
}

func cloneModuleScaffold(in ModuleScaffoldTemplate) ModuleScaffoldTemplate {
	out := in
	out.BestPractices = append([]string{}, in.BestPractices...)
	out.ScaffoldFiles = map[string]string{}
	for path, content := range in.ScaffoldFiles {
		out.ScaffoldFiles[path] = content
	}
	return out
}
