package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleSignatureKeyrings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.signatureAdmission.ListKeys())
	case http.MethodPost:
		var req control.SignatureVerificationKeyInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		key, err := s.signatureAdmission.AddKey(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "security.signature.key.created",
			Message: "signature verification key registered",
			Fields: map[string]any{
				"key_id":    key.ID,
				"algorithm": key.Algorithm,
				"scopes":    key.Scopes,
			},
		}, true)
		writeJSON(w, http.StatusCreated, key)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSignatureKeyringAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/security/signatures/keyrings/{id}
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "security" || parts[2] != "signatures" || parts[3] != "keyrings" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.signatureAdmission.GetKey(parts[4])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "signature key not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleSignatureAdmissionPolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.signatureAdmission.Policy())
	case http.MethodPost:
		var req control.SignatureAdmissionPolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		policy, err := s.signatureAdmission.SetPolicy(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "security.signature.policy.updated",
			Message: "signature admission policy updated",
			Fields: map[string]any{
				"require_signed_scopes": policy.RequireSignedScopes,
				"trusted_key_ids":       policy.TrustedKeyIDs,
			},
		}, true)
		writeJSON(w, http.StatusOK, policy)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSignatureAdmissionCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.SignatureAdmissionInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result := s.signatureAdmission.Admit(req)
	code := http.StatusOK
	if !result.Allowed {
		code = http.StatusConflict
	}
	writeJSON(w, code, result)
}
