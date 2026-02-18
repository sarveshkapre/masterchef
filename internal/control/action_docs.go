package control

import (
	"errors"
	"sort"
	"strings"
)

type ActionDoc struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Endpoints   []string `json:"endpoints"`
	ExampleJSON string   `json:"example_json,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type ActionDocCatalog struct {
	items map[string]ActionDoc
}

func NewActionDocCatalog() *ActionDocCatalog {
	items := []ActionDoc{
		{
			ID:      "bootstrap-workspace",
			Title:   "Bootstrap Workspace Template",
			Summary: "Scaffold a workspace template and optionally create launch artifacts.",
			Endpoints: []string{
				"GET /v1/workspace-templates",
				"POST /v1/workspace-templates/{id}/bootstrap",
			},
			ExampleJSON: `{"output_dir":"./workspace-templates/stateless","create_template":true,"create_runbook":true}`,
			Tags:        []string{"workspace", "bootstrap", "templates"},
		},
		{
			ID:      "execute-bulk-operations",
			Title:   "Preview and Execute Bulk Operations",
			Summary: "Run staged bulk changes with conflict detection and explicit confirmation.",
			Endpoints: []string{
				"POST /v1/bulk/preview",
				"POST /v1/bulk/execute",
			},
			ExampleJSON: `{"name":"bulk-ops","operations":[{"action":"runbook.approve","target_type":"runbook","target_id":"rb-1"}]}`,
			Tags:        []string{"bulk", "change-management", "safety"},
		},
		{
			ID:      "investigate-failed-run",
			Title:   "Investigate Failed Run",
			Summary: "Inspect timeline and triage bundle, then retry or rollback from the same context.",
			Endpoints: []string{
				"GET /v1/runs/{id}/timeline",
				"POST /v1/runs/{id}/triage-bundle",
				"POST /v1/runs/{id}/retry",
				"POST /v1/runs/{id}/rollback",
			},
			ExampleJSON: `{"config_path":"c.yaml","priority":"high"}`,
			Tags:        []string{"incident", "timeline", "rollback"},
		},
		{
			ID:      "incident-correlation",
			Title:   "Correlate Incident Signals",
			Summary: "Aggregate events, alerts, run outcomes, and observability links in one view.",
			Endpoints: []string{
				"GET /v1/incidents/view",
			},
			ExampleJSON: "",
			Tags:        []string{"incident", "alerts", "observability"},
		},
	}
	out := map[string]ActionDoc{}
	for _, item := range items {
		out[item.ID] = item
	}
	return &ActionDocCatalog{items: out}
}

func (c *ActionDocCatalog) List() []ActionDoc {
	out := make([]ActionDoc, 0, len(c.items))
	for _, item := range c.items {
		out = append(out, cloneActionDoc(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (c *ActionDocCatalog) Get(id string) (ActionDoc, error) {
	item, ok := c.items[strings.TrimSpace(id)]
	if !ok {
		return ActionDoc{}, errors.New("action doc not found")
	}
	return cloneActionDoc(item), nil
}

func cloneActionDoc(item ActionDoc) ActionDoc {
	out := item
	out.Endpoints = append([]string{}, item.Endpoints...)
	out.Tags = append([]string{}, item.Tags...)
	return out
}
