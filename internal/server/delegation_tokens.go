package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleDelegationTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.delegationTokens.List())
	case http.MethodPost:
		var req control.DelegationTokenIssueInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		issued, err := s.delegationTokens.Issue(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "access.delegation.issued",
			Message: "time-bound delegation token issued",
			Fields: map[string]any{
				"delegation_id": issued.Delegation.ID,
				"grantor":       issued.Delegation.Grantor,
				"delegatee":     issued.Delegation.Delegatee,
				"pipeline_id":   issued.Delegation.PipelineID,
				"expires_at":    issued.Delegation.ExpiresAt,
			},
		}, true)
		writeJSON(w, http.StatusCreated, issued)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDelegationTokenAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/access/delegation-tokens/{id} or /v1/access/delegation-tokens/{id}/revoke
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "access" || parts[2] != "delegation-tokens" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	switch {
	case len(parts) == 4 && r.Method == http.MethodGet:
		item, ok := s.delegationTokens.Get(parts[3])
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "delegation token not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	case len(parts) == 5 && parts[4] == "revoke" && r.Method == http.MethodPost:
		item, err := s.delegationTokens.Revoke(parts[3])
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "access.delegation.revoked",
			Message: "delegation token revoked",
			Fields: map[string]any{
				"delegation_id": item.ID,
				"grantor":       item.Grantor,
				"delegatee":     item.Delegatee,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDelegationTokenValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.DelegationTokenValidationInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result := s.delegationTokens.Validate(req)
	code := http.StatusOK
	if !result.Allowed {
		code = http.StatusUnauthorized
	}
	writeJSON(w, code, result)
}
