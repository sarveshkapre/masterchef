package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleAdaptiveConcurrencyPolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.adaptiveConcurrency.Policy())
	case http.MethodPost:
		var req control.AdaptiveConcurrencyPolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		policy := s.adaptiveConcurrency.SetPolicy(req)
		writeJSON(w, http.StatusOK, policy)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAdaptiveConcurrencyRecommend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.AdaptiveConcurrencyInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	decision := s.adaptiveConcurrency.Recommend(req)
	writeJSON(w, http.StatusOK, map[string]any{
		"policy":   s.adaptiveConcurrency.Policy(),
		"input":    req,
		"decision": decision,
	})
}
