package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleJITAccessGrants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.jitGrants.List())
	case http.MethodPost:
		var req control.JITAccessGrantIssueInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if id := strings.TrimSpace(req.BreakGlassRequestID); id != "" {
			bg, ok := s.accessApprovals.GetBreakGlassRequest(id)
			if !ok {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "break-glass request not found"})
				return
			}
			if bg.Status != control.BreakGlassActive {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "break-glass request is not active"})
				return
			}
		}
		issued, err := s.jitGrants.Issue(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "access.jit_grant.issued",
			Message: "jit access grant issued",
			Fields: map[string]any{
				"grant_id":               issued.Grant.ID,
				"subject":                issued.Grant.Subject,
				"resource":               issued.Grant.Resource,
				"action":                 issued.Grant.Action,
				"break_glass_request_id": issued.Grant.BreakGlassRequestID,
			},
		}, true)
		writeJSON(w, http.StatusCreated, issued)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleJITAccessGrantAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/access/jit-grants/{id} or /v1/access/jit-grants/{id}/revoke
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "access" || parts[2] != "jit-grants" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	switch {
	case len(parts) == 4 && r.Method == http.MethodGet:
		item, ok := s.jitGrants.Get(parts[3])
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "jit access grant not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	case len(parts) == 5 && parts[4] == "revoke" && r.Method == http.MethodPost:
		item, err := s.jitGrants.Revoke(parts[3])
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "access.jit_grant.revoked",
			Message: "jit access grant revoked",
			Fields: map[string]any{
				"grant_id": item.ID,
				"subject":  item.Subject,
				"resource": item.Resource,
				"action":   item.Action,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleJITAccessGrantValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.JITAccessGrantValidationInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result := s.jitGrants.Validate(req)
	code := http.StatusOK
	if !result.Allowed {
		code = http.StatusUnauthorized
	}
	writeJSON(w, code, result)
}
