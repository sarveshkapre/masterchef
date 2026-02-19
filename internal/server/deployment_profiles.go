package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleDeploymentProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, control.BuiltInDeploymentProfiles())
}

func (s *Server) handleDeploymentProfileEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.DeploymentProfileEvaluationInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	item, err := control.EvaluateDeploymentProfile(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	code := http.StatusOK
	if !item.Pass {
		code = http.StatusConflict
	}
	writeJSON(w, code, item)
}
