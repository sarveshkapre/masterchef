package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleRuntimeSecretSessions(w http.ResponseWriter, r *http.Request) {
	type createReq struct {
		Source     string `json:"source"`
		Passphrase string `json:"passphrase"`
		TTLSeconds int    `json:"ttl_seconds,omitempty"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.runtimeSecrets.List())
	case http.MethodPost:
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		data, _, err := s.encryptedVars.Get(req.Source, req.Passphrase)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		session, err := s.runtimeSecrets.Materialize(control.RuntimeSecretSessionInput{
			Source:     "encrypted-vars:" + req.Source,
			Data:       data,
			TTLSeconds: req.TTLSeconds,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "secrets.runtime.materialized",
			Message: "runtime secret session materialized",
			Fields: map[string]any{
				"session_id":  session.ID,
				"source":      session.Source,
				"ttl_seconds": session.TTLSeconds,
			},
		}, true)
		writeJSON(w, http.StatusCreated, session)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRuntimeSecretConsume(w http.ResponseWriter, r *http.Request) {
	type consumeReq struct {
		SessionID string `json:"session_id"`
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req consumeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	data, session, err := s.runtimeSecrets.Consume(req.SessionID)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "secrets.runtime.consumed",
		Message: "runtime secret session consumed and zeroized",
		Fields: map[string]any{
			"session_id": session.ID,
			"source":     session.Source,
		},
	}, true)
	writeJSON(w, http.StatusOK, map[string]any{
		"session": session,
		"data":    data,
	})
}

func (s *Server) handleRuntimeSecretSessionAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/secrets/runtime/sessions/{id} or /v1/secrets/runtime/sessions/{id}/destroy
	if len(parts) < 5 || parts[0] != "v1" || parts[1] != "secrets" || parts[2] != "runtime" || parts[3] != "sessions" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	sessionID := parts[4]
	switch {
	case len(parts) == 5 && r.Method == http.MethodGet:
		session, ok := s.runtimeSecrets.Get(sessionID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "runtime secret session not found"})
			return
		}
		writeJSON(w, http.StatusOK, session)
	case len(parts) == 6 && parts[5] == "destroy" && r.Method == http.MethodPost:
		session, err := s.runtimeSecrets.Destroy(sessionID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "secrets.runtime.destroyed",
			Message: "runtime secret session destroyed",
			Fields: map[string]any{
				"session_id": session.ID,
				"source":     session.Source,
			},
		}, true)
		writeJSON(w, http.StatusOK, session)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
