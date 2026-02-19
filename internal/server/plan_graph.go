package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/planner"
)

type planGraphNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type"`
	Host  string `json:"host"`
	Order int    `json:"order"`
}

type planGraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

func (s *Server) handlePlanGraph(baseDir string) http.HandlerFunc {
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
		nodes, edges := buildPlanGraph(plan)
		writeJSON(w, http.StatusOK, map[string]any{
			"config_path": configPath,
			"node_count":  len(nodes),
			"edge_count":  len(edges),
			"nodes":       nodes,
			"edges":       edges,
			"dot":         planner.ToDOT(plan),
			"mermaid":     planGraphToMermaid(nodes, edges),
		})
	}
}

func buildPlanGraph(plan *planner.Plan) ([]planGraphNode, []planGraphEdge) {
	if plan == nil {
		return nil, nil
	}
	nodes := make([]planGraphNode, 0, len(plan.Steps))
	edges := make([]planGraphEdge, 0)
	for _, step := range plan.Steps {
		nodes = append(nodes, planGraphNode{
			ID:    step.Resource.ID,
			Label: fmt.Sprintf("%s [%s]", step.Resource.ID, step.Resource.Type),
			Type:  step.Resource.Type,
			Host:  step.Resource.Host,
			Order: step.Order,
		})
		for _, dep := range step.Resource.DependsOn {
			edges = append(edges, planGraphEdge{From: dep, To: step.Resource.ID, Kind: "depends_on"})
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Order != nodes[j].Order {
			return nodes[i].Order < nodes[j].Order
		}
		return nodes[i].ID < nodes[j].ID
	})
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return edges[i].Kind < edges[j].Kind
	})
	return nodes, edges
}

func planGraphToMermaid(nodes []planGraphNode, edges []planGraphEdge) string {
	var b strings.Builder
	b.WriteString("flowchart LR\n")
	for _, node := range nodes {
		label := strings.ReplaceAll(node.Label, `"`, `'`)
		b.WriteString(fmt.Sprintf("  %s[\"%s\"]\n", sanitizeMermaidID(node.ID), label))
	}
	for _, edge := range edges {
		b.WriteString(fmt.Sprintf("  %s --> %s\n", sanitizeMermaidID(edge.From), sanitizeMermaidID(edge.To)))
	}
	return b.String()
}

func sanitizeMermaidID(in string) string {
	if in == "" {
		return "node"
	}
	var b strings.Builder
	for _, r := range in {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "node"
	}
	if out[0] >= '0' && out[0] <= '9' {
		return "node_" + out
	}
	return out
}
