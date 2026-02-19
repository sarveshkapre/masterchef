package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleHealthProbes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.healthProbes.ListTargets())
	case http.MethodPost:
		var req control.HealthProbeTargetInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.healthProbes.UpsertTarget(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "health.probe.target.updated",
			Message: "health probe target updated",
			Fields: map[string]any{
				"probe_target_id": item.ID,
				"name":            item.Name,
				"service":         item.Service,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleHealthProbeChecks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.HealthProbeCheckInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	item, err := s.healthProbes.RecordCheck(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "health.probe.check.recorded",
		Message: "health probe check recorded",
		Fields: map[string]any{
			"probe_check_id": item.ID,
			"target_id":      item.TargetID,
			"status":         item.Status,
			"latency_ms":     item.LatencyMS,
		},
	}, true)
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleHealthProbeGateEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.HealthProbeGateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result := s.healthProbes.EvaluateGate(req)
	writeJSON(w, http.StatusOK, result)
}
