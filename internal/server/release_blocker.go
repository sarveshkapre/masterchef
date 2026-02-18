package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleReleaseBlockerPolicy(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Signals              control.ReadinessSignals     `json:"signals"`
		Thresholds           control.ReadinessThresholds  `json:"thresholds"`
		BaselineAPI          *control.APISpec             `json:"baseline_api,omitempty"`
		SimulationConfidence float64                      `json:"simulation_confidence,omitempty"`
		Policy               control.ReleaseBlockerPolicy `json:"policy,omitempty"`
		ExternalBlockers     []string                     `json:"external_blockers,omitempty"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, control.DefaultReleaseBlockerPolicy())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		readiness := control.EvaluateReadiness(req.Signals, req.Thresholds)
		var diff *control.APIDiffReport
		if req.BaselineAPI != nil {
			cur := currentAPISpec()
			r := control.DiffAPISpec(*req.BaselineAPI, cur)
			diff = &r
		}
		report := control.EvaluateReleaseBlocker(control.ReleaseBlockerInput{
			Readiness:            readiness,
			APIDiff:              diff,
			SimulationConfidence: req.SimulationConfidence,
			ExternalBlockers:     req.ExternalBlockers,
		}, req.Policy)
		resp := map[string]any{
			"report":    report,
			"readiness": readiness,
		}
		if diff != nil {
			resp["api_diff"] = diff
		}
		if !report.Pass {
			writeJSON(w, http.StatusConflict, resp)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
