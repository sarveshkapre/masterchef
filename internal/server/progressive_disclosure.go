package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) handleProgressiveDisclosure(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"profiles": s.progressiveDisclosure.ListProfiles(),
			"active":   s.progressiveDisclosure.ActiveState(),
		})
	case http.MethodPost:
		var req struct {
			ProfileID    string `json:"profile_id"`
			WorkflowHint string `json:"workflow_hint,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		state, err := s.progressiveDisclosure.SetProfile(req.ProfileID, req.WorkflowHint)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, state)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProgressiveDisclosureReveal(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		workflow := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("workflow")))
		state := s.progressiveDisclosure.ActiveState()
		if workflow == "" {
			writeJSON(w, http.StatusOK, map[string]any{
				"active": state,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"workflow": workflow,
			"controls": state.RevealedByFlow[workflow],
			"active":   state,
		})
	case http.MethodPost:
		var req struct {
			Workflow string   `json:"workflow"`
			Controls []string `json:"controls"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		state, err := s.progressiveDisclosure.RevealForWorkflow(req.Workflow, req.Controls)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, state)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
