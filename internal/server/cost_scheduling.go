package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleCostSchedulingPolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.costScheduling.List())
	case http.MethodPost:
		var req control.CostSchedulingPolicyInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.costScheduling.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "control.cost_scheduling.policy.updated",
			Message: "cost-aware scheduling policy updated",
			Fields: map[string]any{
				"policy_id":                item.ID,
				"environment":              item.Environment,
				"max_cost_per_run":         item.MaxCostPerRun,
				"max_hourly_budget":        item.MaxHourlyBudget,
				"off_peak_cost_multiplier": item.OffPeakCostMultiplier,
				"throttle_above_percent":   item.ThrottleAbovePercent,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCostSchedulingAdmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.CostSchedulingAdmissionInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	decision, err := s.costScheduling.Admit(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !decision.Allowed {
		writeJSON(w, http.StatusConflict, decision)
		return
	}
	writeJSON(w, http.StatusOK, decision)
}
