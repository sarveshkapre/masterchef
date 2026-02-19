package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleWorkflowWizards(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items := s.wizards.List()
		if q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q"))); q != "" {
			filtered := make([]control.WorkflowWizard, 0, len(items))
			for _, item := range items {
				text := strings.ToLower(item.ID + " " + item.Title + " " + item.Description + " " + item.UseCase)
				if strings.Contains(text, q) {
					filtered = append(filtered, item)
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"items": filtered,
				"count": len(filtered),
				"query": q,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": items,
			"count": len(items),
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWorkflowWizardAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/wizards/{id}[/launch]
	if len(parts) < 3 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid wizard path"})
		return
	}
	id := strings.TrimSpace(parts[2])
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "wizard id is required"})
		return
	}
	if len(parts) == 3 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, err := s.wizards.Get(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) == 4 && parts[3] == "launch" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Inputs map[string]string `json:"inputs"`
			DryRun bool              `json:"dry_run"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		result, err := s.wizards.Launch(id, control.WorkflowWizardLaunchInput{
			WizardID: id,
			Inputs:   req.Inputs,
			DryRun:   req.DryRun,
		})
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		code := http.StatusOK
		if !result.Ready {
			code = http.StatusAccepted
		}
		writeJSON(w, code, result)
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid wizard action"})
}
