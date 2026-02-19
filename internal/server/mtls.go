package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleMTLSAuthorities(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.mtls.ListAuthorities())
	case http.MethodPost:
		var req control.MTLSAuthorityInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.mtls.CreateAuthority(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "security.mtls.authority.created",
			Message: "mTLS authority created",
			Fields: map[string]any{
				"authority_id": item.ID,
				"name":         item.Name,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMTLSPolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.mtls.ListPolicies())
	case http.MethodPost:
		var req control.MTLSComponentPolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.mtls.SetPolicy(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "security.mtls.policy.updated",
			Message: "mTLS component policy updated",
			Fields: map[string]any{
				"component":           item.Component,
				"min_tls_version":     item.MinTLSVersion,
				"require_client_cert": item.RequireClientCert,
				"allowed_authorities": item.AllowedAuthorities,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMTLSHandshakeCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.MTLSHandshakeCheckInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result := s.mtls.CheckHandshake(req)
	code := http.StatusOK
	if !result.Allowed {
		code = http.StatusConflict
	}
	writeJSON(w, code, result)
}
