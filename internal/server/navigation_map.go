package server

import (
	"net/http"
	"sort"
	"strings"
)

type navigationWorkflow struct {
	Workflow         string   `json:"workflow"`
	ShortcutIDs      []string `json:"shortcut_ids"`
	NoMouseSupported bool     `json:"no_mouse_supported"`
	Description      string   `json:"description,omitempty"`
}

func (s *Server) handleUINavigationMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	workflowFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("workflow")))
	shortcuts := s.shortcuts.List()
	shortcutIndex := map[string]struct{}{}
	for _, item := range shortcuts {
		shortcutIndex[item.ID] = struct{}{}
	}

	workflows := []navigationWorkflow{
		{
			Workflow:         "bootstrap",
			ShortcutIDs:      []string{"command-palette", "runbook-launch"},
			Description:      "Bootstrap flow using palette search and runbook launch.",
			NoMouseSupported: true,
		},
		{
			Workflow:         "rollout",
			ShortcutIDs:      []string{"command-palette", "plans-explain", "apply-run"},
			Description:      "Plan explain + apply execution path for rollout.",
			NoMouseSupported: true,
		},
		{
			Workflow:         "rollback",
			ShortcutIDs:      []string{"command-palette", "runbook-launch"},
			Description:      "Failure rollback using runbook launch path.",
			NoMouseSupported: true,
		},
		{
			Workflow:         "incident-remediation",
			ShortcutIDs:      []string{"incident-view", "command-palette", "runbook-launch"},
			Description:      "Incident view and approved remediation execution.",
			NoMouseSupported: true,
		},
		{
			Workflow:         "drift-management",
			ShortcutIDs:      []string{"drift-insights", "command-palette"},
			Description:      "Drift triage and remediation navigation path.",
			NoMouseSupported: true,
		},
	}

	filtered := make([]navigationWorkflow, 0, len(workflows))
	for _, wf := range workflows {
		if workflowFilter != "" && wf.Workflow != workflowFilter {
			continue
		}
		wf.NoMouseSupported = len(wf.ShortcutIDs) > 0
		for _, id := range wf.ShortcutIDs {
			if _, ok := shortcutIndex[id]; !ok {
				wf.NoMouseSupported = false
				break
			}
		}
		filtered = append(filtered, wf)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Workflow < filtered[j].Workflow })

	supported := 0
	for _, wf := range filtered {
		if wf.NoMouseSupported {
			supported++
		}
	}
	coverage := 0
	if len(filtered) > 0 {
		coverage = (supported * 100) / len(filtered)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"workflows":                 filtered,
		"count":                     len(filtered),
		"no_mouse_coverage_percent": coverage,
		"active_profile":            s.accessibility.ActiveProfile(),
	})
}
