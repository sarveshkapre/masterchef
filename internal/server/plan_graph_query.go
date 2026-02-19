package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/planner"
)

func (s *Server) handlePlanGraphQuery(baseDir string) http.HandlerFunc {
	type reqBody struct {
		ConfigPath string `json:"config_path"`
		ResourceID string `json:"resource_id"`
		Direction  string `json:"direction,omitempty"`
		Depth      int    `json:"depth,omitempty"`
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
		resourceID := strings.TrimSpace(req.ResourceID)
		if configPath == "" || resourceID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path and resource_id are required"})
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
		result := planner.QueryGraph(plan, planner.GraphQueryRequest{
			ResourceID: resourceID,
			Direction:  strings.ToLower(strings.TrimSpace(req.Direction)),
			Depth:      req.Depth,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"config_path": configPath,
			"query": map[string]any{
				"resource_id": resourceID,
				"direction":   strings.ToLower(strings.TrimSpace(req.Direction)),
				"depth":       req.Depth,
			},
			"result": result,
		})
	}
}
