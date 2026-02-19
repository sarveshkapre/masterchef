package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleSSOProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.identity.ListProviders())
	case http.MethodPost:
		var req control.SSOProviderInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.identity.CreateProvider(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "identity.sso.provider.created",
			Message: "sso provider created",
			Fields: map[string]any{
				"provider_id": item.ID,
				"protocol":    item.Protocol,
				"issuer_url":  item.IssuerURL,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSSOProviderAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/identity/sso/providers/{id}[/enable|disable]
	if len(parts) < 5 || parts[0] != "v1" || parts[1] != "identity" || parts[2] != "sso" || parts[3] != "providers" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if len(parts) == 5 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, ok := s.identity.GetProvider(parts[4])
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sso provider not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) == 6 && r.Method == http.MethodPost {
		var enabled bool
		switch parts[5] {
		case "enable":
			enabled = true
		case "disable":
			enabled = false
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown sso provider action"})
			return
		}
		item, err := s.identity.SetProviderEnabled(parts[4], enabled)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
}

func (s *Server) handleSSOLoginStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.SSOLoginStartInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	start, err := s.identity.StartLogin(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, start)
}

func (s *Server) handleSSOLoginCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.SSOLoginCompleteInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	session, err := s.identity.CompleteLogin(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "identity.sso.session.issued",
		Message: "sso session established",
		Fields: map[string]any{
			"session_id":  session.ID,
			"provider_id": session.ProviderID,
			"subject":     session.Subject,
		},
	}, true)
	writeJSON(w, http.StatusCreated, session)
}

func (s *Server) handleSSOSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.identity.ListSessions())
}

func (s *Server) handleSSOSessionAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/identity/sso/sessions/{id}
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "identity" || parts[2] != "sso" || parts[3] != "sessions" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.identity.GetSession(parts[4])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "sso session not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
