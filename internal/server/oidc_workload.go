package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleOIDCWorkloadProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.oidcWorkload.ListProviders())
	case http.MethodPost:
		var req control.OIDCWorkloadProviderInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.oidcWorkload.CreateProvider(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "identity.oidc.provider.created",
			Message: "oidc workload provider configured",
			Fields: map[string]any{
				"provider_id": item.ID,
				"issuer_url":  item.IssuerURL,
				"audience":    item.Audience,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOIDCWorkloadProviderAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/identity/oidc/workload/providers/{id}
	if len(parts) != 6 || parts[0] != "v1" || parts[1] != "identity" || parts[2] != "oidc" || parts[3] != "workload" || parts[4] != "providers" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.oidcWorkload.GetProvider(parts[5])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "oidc workload provider not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleOIDCWorkloadExchange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.OIDCWorkloadExchangeInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	cred, err := s.oidcWorkload.Exchange(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "identity.oidc.credential.issued",
		Message: "oidc workload credential issued",
		Fields: map[string]any{
			"credential_id":   cred.ID,
			"provider_id":     cred.ProviderID,
			"service_account": cred.ServiceAccount,
			"workload":        cred.Workload,
		},
	}, true)
	writeJSON(w, http.StatusCreated, cred)
}

func (s *Server) handleOIDCWorkloadCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.oidcWorkload.ListCredentials())
}

func (s *Server) handleOIDCWorkloadCredentialAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/identity/oidc/workload/credentials/{id}
	if len(parts) != 6 || parts[0] != "v1" || parts[1] != "identity" || parts[2] != "oidc" || parts[3] != "workload" || parts[4] != "credentials" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.oidcWorkload.GetCredential(parts[5])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "oidc workload credential not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
