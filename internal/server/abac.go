package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleABACPolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.abac.ListPolicies())
	case http.MethodPost:
		var req control.ABACPolicyInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.abac.CreatePolicy(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "access.abac.policy.created",
			Message: "abac policy created",
			Fields: map[string]any{
				"policy_id": item.ID,
				"effect":    item.Effect,
				"priority":  item.Priority,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleABACCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.ABACCheckInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result := s.abac.Check(req)
	code := http.StatusOK
	if !result.Allowed {
		code = http.StatusForbidden
	}
	writeJSON(w, code, result)
}
