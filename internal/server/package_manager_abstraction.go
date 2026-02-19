package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handlePackageManagers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.packageManagers.List())
}

func (s *Server) handlePackageManagerResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.PackageManagerResolveInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	result := s.packageManagers.Resolve(req)
	if !result.Compatible {
		writeJSON(w, http.StatusConflict, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePackageManagerRenderAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.PackageManagerActionInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	plan, err := s.packageManagers.RenderAction(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !plan.Allowed {
		writeJSON(w, http.StatusConflict, plan)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}
