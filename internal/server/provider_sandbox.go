package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleProviderSandboxProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.providerSandbox.ListProfiles())
	case http.MethodPost:
		var req control.ProviderSandboxProfileInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.providerSandbox.UpsertProfile(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProviderSandboxProfileAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/providers/sandbox/profiles/{provider}
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "providers" || parts[2] != "sandbox" || parts[3] != "profiles" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.providerSandbox.GetProfile(parts[4])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider sandbox profile not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleProviderSandboxEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.ProviderSandboxEvaluateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	eval := s.providerSandbox.Evaluate(req)
	if !eval.Allowed {
		writeJSON(w, http.StatusConflict, eval)
		return
	}
	writeJSON(w, http.StatusOK, eval)
}
