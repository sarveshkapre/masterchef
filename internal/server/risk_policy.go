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

type policyDecision struct {
	Order      int    `json:"order"`
	ResourceID string `json:"resource_id"`
	Resource   string `json:"resource"`
	Host       string `json:"host"`
	Allowed    bool   `json:"allowed"`
	Reason     string `json:"reason"`
}

func (s *Server) handlePolicySimulation(baseDir string) http.HandlerFunc {
	type reqBody struct {
		ConfigPath        string   `json:"config_path"`
		DenyResourceTypes []string `json:"deny_resource_types,omitempty"`
		DenyHosts         []string `json:"deny_hosts,omitempty"`
		MinimumConfidence float64  `json:"minimum_confidence,omitempty"`
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
		configPath, cfg, p, err := loadPlanRequest(baseDir, req.ConfigPath)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		denyTypes := normalizeStringSet(req.DenyResourceTypes)
		denyHosts := normalizeStringSet(req.DenyHosts)
		minConfidence := req.MinimumConfidence
		if minConfidence <= 0 {
			minConfidence = 0.5
		}
		if minConfidence > 1 {
			minConfidence = 1
		}

		decisions := make([]policyDecision, 0, len(p.Steps))
		supported := 0
		for _, step := range p.Steps {
			allowed := true
			reason := "allowed by simulation guardrails"
			stepType := strings.ToLower(strings.TrimSpace(step.Resource.Type))
			stepHost := strings.ToLower(strings.TrimSpace(step.Resource.Host))
			if _, denied := denyTypes[stepType]; denied {
				allowed = false
				reason = "blocked: resource type denied by simulation policy"
			}
			if _, denied := denyHosts[stepHost]; denied {
				allowed = false
				reason = "blocked: host denied by simulation policy"
			}
			if simulationSupported(stepType) {
				supported++
			}
			decisions = append(decisions, policyDecision{
				Order:      step.Order,
				ResourceID: step.Resource.ID,
				Resource:   step.Resource.Type,
				Host:       step.Resource.Host,
				Allowed:    allowed,
				Reason:     reason,
			})
		}
		confidence := 1.0
		if len(p.Steps) > 0 {
			confidence = float64(supported) / float64(len(p.Steps))
		}
		blockReasons := make([]string, 0)
		blockedByPolicy := false
		for _, d := range decisions {
			if !d.Allowed {
				blockedByPolicy = true
				blockReasons = append(blockReasons, d.ResourceID+": "+d.Reason)
			}
		}
		if confidence < minConfidence {
			blockReasons = append(blockReasons, "simulation confidence below required minimum threshold")
		}
		mitigations := []string{
			"review denied steps and update policy guardrails if needed",
			"run check/noop mode to validate non-blocked changes",
			"split high-risk resources into smaller rollout batches",
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"config_path":           configPath,
			"resource_count":        len(cfg.Resources),
			"step_count":            len(p.Steps),
			"simulation_confidence": confidence,
			"minimum_confidence":    minConfidence,
			"coverage": map[string]any{
				"supported_steps":   supported,
				"unsupported_steps": len(p.Steps) - supported,
				"total_steps":       len(p.Steps),
			},
			"decisions":         decisions,
			"would_block_apply": blockedByPolicy || confidence < minConfidence,
			"block_reasons":     blockReasons,
			"mitigations":       mitigations,
		})
	}
}

func (s *Server) handlePlanRiskSummary(baseDir string) http.HandlerFunc {
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
		configPath, _, p, err := loadPlanRequest(baseDir, req.ConfigPath)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		radius := planner.AnalyzeBlastRadius(p)
		score, drivers, mitigations := riskFromPlan(p, radius)
		level := "low"
		switch {
		case score >= 70:
			level = "high"
		case score >= 35:
			level = "medium"
		}
		summary := "Risk is low with mostly deterministic, low-blast-radius changes."
		if level == "medium" {
			summary = "Risk is moderate; review dependency order and stage rollout before apply."
		}
		if level == "high" {
			summary = "Risk is high; require approvals and phased rollout with rollback readiness."
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"config_path": configPath,
			"risk_score":  score,
			"risk_level":  level,
			"summary":     summary,
			"drivers":     drivers,
			"mitigations": mitigations,
			"blast_radius": map[string]any{
				"total_steps":     radius.TotalSteps,
				"affected_hosts":  radius.AffectedHosts,
				"affected_types":  radius.AffectedTypes,
				"estimated_scope": radius.EstimatedScope,
			},
		})
	}
}

func loadPlanRequest(baseDir, configPath string) (string, *config.Config, *planner.Plan, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return "", nil, nil, os.ErrInvalid
	}
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(baseDir, configPath)
	}
	if _, err := os.Stat(configPath); err != nil {
		return "", nil, nil, err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return "", nil, nil, err
	}
	p, err := planner.Build(cfg)
	if err != nil {
		return "", nil, nil, err
	}
	return configPath, cfg, p, nil
}

func simulationSupported(resourceType string) bool {
	switch strings.ToLower(strings.TrimSpace(resourceType)) {
	case "file", "command":
		return true
	default:
		return false
	}
}

func normalizeStringSet(items []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		out[item] = struct{}{}
	}
	return out
}

func riskFromPlan(p *planner.Plan, radius planner.BlastRadius) (int, []string, []string) {
	score := 0
	drivers := make([]string, 0, 4)
	commandCount := 0
	resourceTypes := map[string]struct{}{}
	for _, step := range p.Steps {
		typ := strings.ToLower(strings.TrimSpace(step.Resource.Type))
		resourceTypes[typ] = struct{}{}
		if typ == "command" {
			commandCount++
		}
	}
	if commandCount > 0 {
		score += commandCount * 15
		drivers = append(drivers, "command resources introduce imperative side-effect risk")
	}
	if radius.EstimatedScope == "high" {
		score += 30
		drivers = append(drivers, "blast radius scope is high")
	} else if radius.EstimatedScope == "medium" {
		score += 15
		drivers = append(drivers, "blast radius scope is medium")
	}
	if len(radius.AffectedHosts) > 3 {
		score += 10
		drivers = append(drivers, "multiple hosts are affected in one apply")
	}
	if len(resourceTypes) > 2 {
		score += 10
		drivers = append(drivers, "mixed resource types increase operational complexity")
	}
	if score > 100 {
		score = 100
	}
	sort.Strings(drivers)
	mitigations := []string{
		"run plan explainability and blast-radius review before apply",
		"use maintenance windows and approval gates for medium/high risk plans",
		"stage rollout by host groups and monitor incident view during execution",
	}
	return score, drivers, mitigations
}
