package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleArtifactDeployments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.artifactDeployments.List())
	case http.MethodPost:
		var req control.ArtifactDeploymentInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, plan, err := s.artifactDeployments.Create(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		code := http.StatusCreated
		if !plan.Allowed {
			code = http.StatusConflict
		}
		writeJSON(w, code, map[string]any{
			"deployment": item,
			"plan":       plan,
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleArtifactDeploymentAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/execution/artifacts/deployments/{id}[/plan]
	if len(parts) < 5 || parts[0] != "v1" || parts[1] != "execution" || parts[2] != "artifacts" || parts[3] != "deployments" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := parts[4]
	if len(parts) == 5 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, ok := s.artifactDeployments.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "artifact deployment not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) == 6 && parts[5] == "plan" {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		plan, err := s.artifactDeployments.PlanByID(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		code := http.StatusOK
		if !plan.Allowed {
			code = http.StatusConflict
		}
		writeJSON(w, code, plan)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}
