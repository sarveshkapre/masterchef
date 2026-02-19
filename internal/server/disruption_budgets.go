package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleDisruptionBudgets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.disruptionBudgets.List())
	case http.MethodPost:
		var req control.DisruptionBudgetInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.disruptionBudgets.Create(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "control.disruption_budget.created",
			Message: "disruption budget created",
			Fields: map[string]any{
				"budget_id":       item.ID,
				"name":            item.Name,
				"max_unavailable": item.MaxUnavailable,
				"min_healthy_pct": item.MinHealthyPct,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDisruptionBudgetEvaluate(w http.ResponseWriter, r *http.Request) {
	type evalReq struct {
		BudgetID             string `json:"budget_id"`
		TotalTargets         int    `json:"total_targets"`
		RequestedDisruptions int    `json:"requested_disruptions"`
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req evalReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	budget, ok := s.disruptionBudgets.Get(req.BudgetID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "disruption budget not found"})
		return
	}
	report := control.EvaluateDisruptionBudget(budget, req.TotalTargets, req.RequestedDisruptions)
	s.recordEvent(control.Event{
		Type:    "control.disruption_budget.evaluated",
		Message: "disruption budget evaluated",
		Fields: map[string]any{
			"budget_id":     report.BudgetID,
			"allowed":       report.Allowed,
			"total_targets": report.TotalTargets,
			"requested":     report.RequestedDisruptions,
			"healthy_pct":   report.HealthyPctAfter,
		},
	}, true)
	code := http.StatusOK
	if !report.Allowed {
		code = http.StatusConflict
	}
	writeJSON(w, code, report)
}
