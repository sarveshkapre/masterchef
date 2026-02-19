package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleDelegatedAdminGrants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.delegatedAdmin.List())
	case http.MethodPost:
		var req control.DelegatedAdminGrantInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.delegatedAdmin.Create(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "control.delegated_admin.grant.created",
			Message: "delegated admin grant created",
			Fields: map[string]any{
				"grant_id":    item.ID,
				"tenant":      item.Tenant,
				"environment": item.Environment,
				"principal":   item.Principal,
				"scopes":      item.Scopes,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDelegatedAdminAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.DelegatedAdminAuthorizeInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	decision := s.delegatedAdmin.Authorize(req)
	if !decision.Allowed {
		writeJSON(w, http.StatusForbidden, decision)
		return
	}
	writeJSON(w, http.StatusOK, decision)
}
