package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleExecutionCredentials(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.executionCreds.List())
	case http.MethodPost:
		var req control.ExecutionCredentialIssueInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		issued, err := s.executionCreds.Issue(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "execution.credential.issued",
			Message: "short-lived execution credential issued",
			Fields: map[string]any{
				"credential_id": issued.Credential.ID,
				"subject":       issued.Credential.Subject,
				"ttl_seconds":   issued.Credential.TTLSeconds,
			},
		}, true)
		writeJSON(w, http.StatusCreated, issued)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleExecutionCredentialAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/execution/credentials/{id}
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "execution" || parts[2] != "credentials" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	switch {
	case len(parts) == 4 && r.Method == http.MethodGet:
		item, ok := s.executionCreds.Get(parts[3])
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "execution credential not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	case len(parts) == 5 && parts[4] == "revoke" && r.Method == http.MethodPost:
		item, err := s.executionCreds.Revoke(parts[3])
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "execution.credential.revoked",
			Message: "execution credential revoked",
			Fields: map[string]any{
				"credential_id": item.ID,
				"subject":       item.Subject,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleExecutionCredentialValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.ExecutionCredentialValidationInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result := s.executionCreds.Validate(req)
	code := http.StatusOK
	if !result.Allowed {
		code = http.StatusUnauthorized
	}
	writeJSON(w, code, result)
}
