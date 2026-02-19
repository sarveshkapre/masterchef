package control

import (
	"sort"
	"strings"
)

type UIShortcut struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Category       string `json:"category"`
	Keystroke      string `json:"keystroke"`
	ActionEndpoint string `json:"action_endpoint"`
	Description    string `json:"description,omitempty"`
	Global         bool   `json:"global"`
}

type UIShortcutCatalog struct {
	items map[string]UIShortcut
}

func NewUIShortcutCatalog() *UIShortcutCatalog {
	seed := []UIShortcut{
		{ID: "command-palette", Title: "Open Command Palette", Category: "navigation", Keystroke: "CmdOrCtrl+K", ActionEndpoint: "GET /v1/search", Description: "Search hosts, services, runs, and policies.", Global: true},
		{ID: "plans-explain", Title: "Explain Current Plan", Category: "planning", Keystroke: "Shift+E", ActionEndpoint: "POST /v1/plans/explain", Global: false},
		{ID: "apply-run", Title: "Start Apply", Category: "execution", Keystroke: "Shift+A", ActionEndpoint: "POST /v1/apply", Global: false},
		{ID: "drift-insights", Title: "Open Drift Insights", Category: "drift", Keystroke: "Shift+D", ActionEndpoint: "GET /v1/drift/insights", Global: true},
		{ID: "incident-view", Title: "Open Incident View", Category: "incident", Keystroke: "Shift+I", ActionEndpoint: "GET /v1/incidents/view", Global: true},
		{ID: "runbook-launch", Title: "Launch Approved Runbook", Category: "operations", Keystroke: "Shift+R", ActionEndpoint: "POST /v1/runbooks/{id}/launch", Global: false},
	}
	index := make(map[string]UIShortcut, len(seed))
	for _, item := range seed {
		index[item.ID] = item
	}
	return &UIShortcutCatalog{items: index}
}

func (c *UIShortcutCatalog) List() []UIShortcut {
	out := make([]UIShortcut, 0, len(c.items))
	for _, item := range c.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (c *UIShortcutCatalog) Search(query string) []UIShortcut {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return c.List()
	}
	items := c.List()
	filtered := make([]UIShortcut, 0, len(items))
	for _, item := range items {
		text := strings.ToLower(item.ID + " " + item.Title + " " + item.Category + " " + item.Keystroke + " " + item.Description + " " + item.ActionEndpoint)
		if strings.Contains(text, q) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
