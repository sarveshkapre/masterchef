package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/planner"
)

type planDiffPreviewItem struct {
	Order      int    `json:"order"`
	ResourceID string `json:"resource_id"`
	Type       string `json:"type"`
	Host       string `json:"host"`
	Changed    bool   `json:"changed"`
	Preview    string `json:"preview"`
}

func (s *Server) handlePlanDiffPreview(baseDir string) http.HandlerFunc {
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
		items, changed := buildPlanDiffPreview(plan, baseDir)
		writeJSON(w, http.StatusOK, map[string]any{
			"config_path":     configPath,
			"step_count":      len(items),
			"changed_actions": changed,
			"items":           items,
		})
	}
}

func buildPlanDiffPreview(plan *planner.Plan, baseDir string) ([]planDiffPreviewItem, int) {
	if plan == nil {
		return nil, 0
	}
	items := make([]planDiffPreviewItem, 0, len(plan.Steps))
	changed := 0
	for _, step := range plan.Steps {
		item := planDiffPreviewItem{
			Order:      step.Order,
			ResourceID: step.Resource.ID,
			Type:       step.Resource.Type,
			Host:       step.Resource.Host,
		}
		switch strings.ToLower(strings.TrimSpace(step.Resource.Type)) {
		case "file":
			path := strings.TrimSpace(step.Resource.Path)
			if path != "" && !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			current, readErr := os.ReadFile(path)
			desired := []byte(step.Resource.Content)
			if readErr != nil {
				item.Changed = true
				item.Preview = fmt.Sprintf("create file %s with %d bytes", path, len(desired))
			} else if string(current) != string(desired) {
				item.Changed = true
				item.Preview = renderInlineDiff(string(current), string(desired), 6)
			} else {
				item.Changed = false
				item.Preview = "no change"
			}
		case "command":
			item.Changed = true
			parts := []string{fmt.Sprintf("run command: %s", strings.TrimSpace(step.Resource.Command))}
			if strings.TrimSpace(step.Resource.RefreshCommand) != "" {
				parts = append(parts, "refresh command: "+strings.TrimSpace(step.Resource.RefreshCommand))
			}
			if step.Resource.RefreshOnly {
				parts = append(parts, "refresh_only: true")
			}
			if strings.TrimSpace(step.Resource.Creates) != "" {
				parts = append(parts, "creates guard: "+strings.TrimSpace(step.Resource.Creates))
			}
			if strings.TrimSpace(step.Resource.OnlyIf) != "" {
				parts = append(parts, "only_if guard: "+strings.TrimSpace(step.Resource.OnlyIf))
			}
			if strings.TrimSpace(step.Resource.Unless) != "" {
				parts = append(parts, "unless guard: "+strings.TrimSpace(step.Resource.Unless))
			}
			if len(step.Resource.NotifyHandlers) > 0 {
				parts = append(parts, "notify_handlers: "+strings.Join(step.Resource.NotifyHandlers, ","))
			}
			item.Preview = strings.Join(parts, " | ")
		default:
			item.Changed = true
			item.Preview = "preview unavailable for resource type"
		}
		if item.Changed {
			changed++
		}
		items = append(items, item)
	}
	return items, changed
}

func renderInlineDiff(current, desired string, maxLines int) string {
	curLines := strings.Split(current, "\n")
	desLines := strings.Split(desired, "\n")
	lines := make([]string, 0, maxLines)
	lines = append(lines, "--- current", "+++ desired")
	limit := len(curLines)
	if len(desLines) > limit {
		limit = len(desLines)
	}
	for i := 0; i < limit && len(lines) < maxLines; i++ {
		var c, d string
		if i < len(curLines) {
			c = curLines[i]
		}
		if i < len(desLines) {
			d = desLines[i]
		}
		if c == d {
			continue
		}
		if c != "" && len(lines) < maxLines {
			lines = append(lines, "-"+c)
		}
		if d != "" && len(lines) < maxLines {
			lines = append(lines, "+"+d)
		}
	}
	if len(lines) == 2 {
		return "content changed"
	}
	return strings.Join(lines, "\n")
}
