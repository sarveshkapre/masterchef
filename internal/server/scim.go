package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleSCIMRoles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.scim.ListRoles())
	case http.MethodPost:
		var req control.SCIMRoleInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.scim.UpsertRole(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "identity.scim.role.upserted",
			Message: "scim role provisioned",
			Fields: map[string]any{
				"role_id":     item.ID,
				"external_id": item.ExternalID,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSCIMRoleAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/identity/scim/roles/{id}
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "identity" || parts[2] != "scim" || parts[3] != "roles" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.scim.GetRole(parts[4])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "scim role not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleSCIMTeams(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.scim.ListTeams())
	case http.MethodPost:
		var req control.SCIMTeamInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.scim.UpsertTeam(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "identity.scim.team.upserted",
			Message: "scim team provisioned",
			Fields: map[string]any{
				"team_id":     item.ID,
				"external_id": item.ExternalID,
				"members":     len(item.Members),
				"roles":       item.Roles,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSCIMTeamAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/identity/scim/teams/{id}
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "identity" || parts[2] != "scim" || parts[3] != "teams" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.scim.GetTeam(parts[4])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "scim team not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
