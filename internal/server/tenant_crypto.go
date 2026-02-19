package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleTenantCryptoKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.tenantCrypto.List())
	case http.MethodPost:
		var req control.TenantCryptoKeyInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.tenantCrypto.EnsureTenantKey(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "security.tenant_keys.ensure",
			Message: "tenant key ensured",
			Fields: map[string]any{
				"key_id":      item.ID,
				"tenant":      item.Tenant,
				"algorithm":   item.Algorithm,
				"version":     item.Version,
				"fingerprint": item.Fingerprint,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTenantCryptoRotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.TenantKeyRotateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	item, err := s.tenantCrypto.Rotate(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "security.tenant_keys.rotate",
		Message: "tenant key rotated",
		Fields: map[string]any{
			"key_id":    item.ID,
			"tenant":    item.Tenant,
			"algorithm": item.Algorithm,
			"version":   item.Version,
		},
	}, true)
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleTenantCryptoBoundaryCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.TenantBoundaryCheckInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	decision := s.tenantCrypto.BoundaryCheck(req)
	if !decision.Allowed {
		writeJSON(w, http.StatusConflict, decision)
		return
	}
	writeJSON(w, http.StatusOK, decision)
}
