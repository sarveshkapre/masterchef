package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/planner"
)

type planExplainItem struct {
	Order           int      `json:"order"`
	ResourceID      string   `json:"resource_id"`
	ResourceType    string   `json:"resource_type"`
	Host            string   `json:"host"`
	Dependencies    []string `json:"dependencies,omitempty"`
	Reason          string   `json:"reason"`
	TriggeredBy     string   `json:"triggered_by"`
	ExpectedOutcome string   `json:"expected_outcome"`
	RiskHint        string   `json:"risk_hint"`
}

func (s *Server) handlePlanExplain(baseDir string) http.HandlerFunc {
	type reqBody struct {
		ConfigPath string `json:"config_path"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		configPath := strings.TrimSpace(req.ConfigPath)
		if configPath == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path is required"})
			return
		}
		if !filepath.IsAbs(configPath) {
			configPath = filepath.Join(baseDir, configPath)
		}
		if _, err := os.Stat(configPath); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path not found"})
			return
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		plan, err := planner.Build(cfg)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		items := explainPlan(cfg, plan)
		summary := explainSummary(items)
		writeJSON(w, http.StatusOK, map[string]any{
			"config_path": configPath,
			"summary":     summary,
			"steps":       items,
		})
	}
}

func explainPlan(cfg *config.Config, p *planner.Plan) []planExplainItem {
	if cfg == nil || p == nil {
		return nil
	}
	resByID := map[string]config.Resource{}
	for _, res := range cfg.Resources {
		resByID[res.ID] = res
	}
	items := make([]planExplainItem, 0, len(p.Steps))
	for _, step := range p.Steps {
		res := resByID[step.Resource.ID]
		deps := append([]string{}, res.DependsOn...)
		sort.Strings(deps)
		reason := "scheduled because resource is declared in desired state"
		if len(deps) > 0 {
			reason = "scheduled after dependencies: " + strings.Join(deps, ", ")
		}
		items = append(items, planExplainItem{
			Order:           step.Order,
			ResourceID:      step.Resource.ID,
			ResourceType:    step.Resource.Type,
			Host:            step.Resource.Host,
			Dependencies:    deps,
			Reason:          reason,
			TriggeredBy:     "config resource declaration and dependency graph",
			ExpectedOutcome: expectedOutcomeForResource(step.Resource.Type),
			RiskHint:        riskHintForResource(step.Resource.Type),
		})
	}
	return items
}

func expectedOutcomeForResource(resourceType string) string {
	switch strings.ToLower(strings.TrimSpace(resourceType)) {
	case "file", "template":
		return "file content and metadata converge to desired value"
	case "package":
		return "package presence/version converges to desired state"
	case "service":
		return "service state converges (running/enabled/restarted if changed)"
	case "command":
		return "command guard conditions evaluated and command executed only if required"
	default:
		return "resource converges to desired state if drift is detected"
	}
}

func riskHintForResource(resourceType string) string {
	switch strings.ToLower(strings.TrimSpace(resourceType)) {
	case "command":
		return "higher risk: imperative command resources may have side effects"
	case "service":
		return "medium risk: service restarts can cause transient impact"
	case "package":
		return "medium risk: package upgrades may require compatibility checks"
	default:
		return "low risk: deterministic convergence expected when idempotent"
	}
}

func explainSummary(items []planExplainItem) map[string]any {
	highRisk := 0
	mediumRisk := 0
	for _, item := range items {
		risk := strings.ToLower(item.RiskHint)
		switch {
		case strings.Contains(risk, "higher risk"):
			highRisk++
		case strings.Contains(risk, "medium risk"):
			mediumRisk++
		}
	}
	return map[string]any{
		"step_count":        len(items),
		"high_risk_steps":   highRisk,
		"medium_risk_steps": mediumRisk,
		"recommended_actions": []string{
			"review high-risk steps before apply",
			"validate dependency ordering against maintenance windows",
			"use check/noop mode for pre-apply confidence",
		},
	}
}
