package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleRegionalFailoverDrills(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Regions             []string `json:"regions"`
		TargetRTOSeconds    int      `json:"target_rto_seconds"`
		SimulatedRecoveryMs int64    `json:"simulated_recovery_ms"`
		Notes               string   `json:"notes"`
	}
	switch r.Method {
	case http.MethodGet:
		limit := 100
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		writeJSON(w, http.StatusOK, s.failoverDrills.List(limit))
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		regions := req.Regions
		if len(regions) == 0 {
			regions = []string{"global"}
		}
		runs := make([]control.RegionalFailoverDrillRun, 0, len(regions))
		for _, region := range regions {
			run, err := s.failoverDrills.Run(control.RegionalFailoverDrillInput{
				Region:              region,
				TargetRTOSeconds:    req.TargetRTOSeconds,
				SimulatedRecoveryMs: req.SimulatedRecoveryMs,
				Notes:               req.Notes,
			})
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			runs = append(runs, run)
			s.recordEvent(control.Event{
				Type:    "control.failover.drill",
				Message: "regional failover drill executed",
				Fields: map[string]any{
					"run_id":           run.ID,
					"region":           run.Region,
					"pass":             run.Pass,
					"recovery_time_ms": run.RecoveryTimeMs,
					"target_rto_ms":    run.TargetRTOMs,
				},
			}, true)
		}
		writeJSON(w, http.StatusOK, map[string]any{"runs": runs, "count": len(runs)})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRegionalFailoverScorecards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	windowHours := 24 * 30
	if raw := strings.TrimSpace(r.URL.Query().Get("window_hours")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			windowHours = n
		}
	}
	writeJSON(w, http.StatusOK, s.failoverDrills.Scorecards(windowHours))
}
