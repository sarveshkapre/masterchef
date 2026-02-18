package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleRoles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.roleEnv.ListRoles())
	case http.MethodPost:
		var req control.RoleDefinition
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		_, exists := s.roleEnv.GetRole(req.Name)
		item, err := s.roleEnv.UpsertRole(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		status := http.StatusCreated
		if exists == nil {
			status = http.StatusOK
		}
		writeJSON(w, status, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRoleAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) < 3 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid role path"})
		return
	}
	name := parts[2]
	if len(parts) == 3 {
		switch r.Method {
		case http.MethodGet:
			item, err := s.roleEnv.GetRole(name)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, item)
		case http.MethodDelete:
			if !s.roleEnv.DeleteRole(name) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}

	action := parts[3]
	if action != "resolve" || r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	envName := r.URL.Query().Get("environment")
	if envName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment query parameter is required"})
		return
	}
	resolution, err := s.roleEnv.Resolve(name, envName)
	if err != nil {
		if err.Error() == "role not found" || err.Error() == "environment not found" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resolution)
}

func (s *Server) handleEnvironments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.roleEnv.ListEnvironments())
	case http.MethodPost:
		var req control.EnvironmentDefinition
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		_, exists := s.roleEnv.GetEnvironment(req.Name)
		item, err := s.roleEnv.UpsertEnvironment(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		status := http.StatusCreated
		if exists == nil {
			status = http.StatusOK
		}
		writeJSON(w, status, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEnvironmentAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) < 3 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid environment path"})
		return
	}
	name := parts[2]
	switch r.Method {
	case http.MethodGet:
		item, err := s.roleEnv.GetEnvironment(name)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		if !s.roleEnv.DeleteEnvironment(name) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "environment not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
