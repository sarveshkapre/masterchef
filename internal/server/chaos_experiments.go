package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleChaosExperiments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"items": s.chaosExperiments.List()})
	case http.MethodPost:
		var req control.ChaosExperimentInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.chaosExperiments.Create(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleChaosExperimentAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/control/chaos/experiments/{id}[/abort|complete]
	if len(parts) < 5 || len(parts) > 6 || !strings.EqualFold(parts[0], "v1") || !strings.EqualFold(parts[1], "control") || !strings.EqualFold(parts[2], "chaos") || !strings.EqualFold(parts[3], "experiments") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid chaos experiment path"})
		return
	}
	id := strings.TrimSpace(parts[4])
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "experiment id is required"})
		return
	}
	if len(parts) == 5 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, err := s.chaosExperiments.Get(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	switch strings.ToLower(strings.TrimSpace(parts[5])) {
	case "abort":
		var req struct {
			Reason string `json:"reason"`
		}
		if r.ContentLength > 0 {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
		item, err := s.chaosExperiments.Abort(id, req.Reason)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	case "complete":
		item, err := s.chaosExperiments.Complete(id)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown chaos experiment action"})
	}
}
