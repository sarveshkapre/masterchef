package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleFIPSMode(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.fipsMode.Get())
	case http.MethodPost:
		var req control.FIPSModeInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		mode, err := s.fipsMode.Set(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "security.fips.mode.updated",
			Message: "fips cryptography mode updated",
			Fields: map[string]any{
				"enabled":                      mode.Enabled,
				"min_rsa_key_bits":             mode.MinRSAKeyBits,
				"allowed_signature_algorithms": mode.AllowedSignatureAlgorithms,
				"allowed_hash_algorithms":      mode.AllowedHashAlgorithms,
				"block_legacy_tls":             mode.BlockLegacyTLS,
			},
		}, true)
		writeJSON(w, http.StatusOK, mode)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleFIPSValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.FIPSValidationInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	result := s.fipsMode.Validate(req)
	code := http.StatusOK
	if !result.Allowed {
		code = http.StatusConflict
	}
	writeJSON(w, code, result)
}
