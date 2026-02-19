package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleUpgradeOrchestrationPlans(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.upgradeOrchestration.ListPlans())
	case http.MethodPost:
		var req control.UpgradeOrchestrationPlanInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.upgradeOrchestration.CreatePlan(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "control.upgrade.orchestration.plan.created",
			Message: "upgrade orchestration plan created",
			Fields: map[string]any{
				"plan_id":      item.ID,
				"component":    item.Component,
				"from_channel": item.FromChannel,
				"to_channel":   item.ToChannel,
				"strategy":     item.Strategy,
				"total_nodes":  item.TotalNodes,
				"wave_size":    item.WaveSize,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUpgradeOrchestrationPlanAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/control/upgrade-orchestration/plans/{id}[/advance|abort]
	if len(parts) < 5 || parts[0] != "v1" || parts[1] != "control" || parts[2] != "upgrade-orchestration" || parts[3] != "plans" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := parts[4]
	if len(parts) == 5 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, ok := s.upgradeOrchestration.GetPlan(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "upgrade plan not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	action := strings.ToLower(parts[5])
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "advance":
		var req control.UpgradeOrchestrationAdvanceInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.upgradeOrchestration.Advance(id, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if item.Status == "blocked" {
			writeJSON(w, http.StatusConflict, item)
			return
		}
		writeJSON(w, http.StatusOK, item)
	case "abort":
		var req control.UpgradeOrchestrationAbortInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.upgradeOrchestration.Abort(id, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}
