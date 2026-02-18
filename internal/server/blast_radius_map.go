package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/planner"
)

type blastNode struct {
	ID       string         `json:"id"`
	Kind     string         `json:"kind"` // host|resource
	Label    string         `json:"label"`
	Risk     string         `json:"risk,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type blastEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Kind  string `json:"kind"` // contains|depends_on
	Label string `json:"label,omitempty"`
}

func (s *Server) handleBlastRadiusMap(baseDir string) http.HandlerFunc {
	type reqBody struct {
		ConfigPath string            `json:"config_path"`
		Owners     map[string]string `json:"owners,omitempty"` // host/resource -> owner
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
		p, err := planner.Build(cfg)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		radius := planner.AnalyzeBlastRadius(p)
		nodes, edges := buildBlastGraph(cfg, req.Owners)
		writeJSON(w, http.StatusOK, map[string]any{
			"config_path":  configPath,
			"analysis":     radius,
			"graph":        map[string]any{"nodes": nodes, "edges": edges},
			"risk_summary": blastRiskSummary(radius),
			"generated_at": time.Now().UTC(),
		})
	}
}

func buildBlastGraph(cfg *config.Config, owners map[string]string) ([]blastNode, []blastEdge) {
	nodes := make([]blastNode, 0, len(cfg.Inventory.Hosts)+len(cfg.Resources))
	edges := make([]blastEdge, 0, len(cfg.Resources)*2)

	for _, host := range cfg.Inventory.Hosts {
		owner := strings.TrimSpace(owners[host.Name])
		node := blastNode{
			ID:    "host:" + host.Name,
			Kind:  "host",
			Label: host.Name,
			Risk:  "low",
			Metadata: map[string]any{
				"transport": host.Transport,
				"owner":     owner,
			},
		}
		nodes = append(nodes, node)
	}
	for _, res := range cfg.Resources {
		risk := "low"
		switch strings.ToLower(strings.TrimSpace(res.Type)) {
		case "command":
			risk = "high"
		case "package", "service":
			risk = "medium"
		}
		owner := strings.TrimSpace(owners[res.ID])
		if owner == "" {
			owner = strings.TrimSpace(owners[res.Host])
		}
		resourceNodeID := "resource:" + res.ID
		nodes = append(nodes, blastNode{
			ID:    resourceNodeID,
			Kind:  "resource",
			Label: res.ID,
			Risk:  risk,
			Metadata: map[string]any{
				"type":  res.Type,
				"host":  res.Host,
				"owner": owner,
			},
		})
		edges = append(edges, blastEdge{
			From: "host:" + res.Host,
			To:   resourceNodeID,
			Kind: "contains",
		})
		for _, dep := range res.DependsOn {
			edges = append(edges, blastEdge{
				From:  "resource:" + dep,
				To:    resourceNodeID,
				Kind:  "depends_on",
				Label: dep + " -> " + res.ID,
			})
		}
	}
	return nodes, edges
}

func blastRiskSummary(radius planner.BlastRadius) map[string]any {
	return map[string]any{
		"estimated_scope": radius.EstimatedScope,
		"total_steps":     radius.TotalSteps,
		"affected_hosts":  len(radius.AffectedHosts),
		"affected_types":  len(radius.AffectedTypes),
		"message":         "Review host/resource dependency graph before apply to minimize blast radius.",
	}
}
