package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleRBACRoles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.rbac.ListRoles())
	case http.MethodPost:
		var req control.RBACRoleInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.rbac.CreateRole(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "access.rbac.role.created",
			Message: "rbac role created",
			Fields: map[string]any{
				"role_id":     item.ID,
				"name":        item.Name,
				"permissions": item.Permissions,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRBACRoleAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/access/rbac/roles/{id}
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "access" || parts[2] != "rbac" || parts[3] != "roles" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.rbac.GetRole(parts[4])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "rbac role not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleRBACBindings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.rbac.ListBindings())
	case http.MethodPost:
		var req control.RBACBindingInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.rbac.CreateBinding(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "access.rbac.binding.created",
			Message: "rbac role binding created",
			Fields: map[string]any{
				"binding_id": item.ID,
				"subject":    item.Subject,
				"role_id":    item.RoleID,
				"scope":      item.Scope,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRBACAccessCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.RBACAccessCheckInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result := s.rbac.CheckAccess(req)
	code := http.StatusOK
	if !result.Allowed {
		code = http.StatusForbidden
	}
	writeJSON(w, code, result)
}
