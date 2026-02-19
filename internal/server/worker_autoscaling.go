package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleWorkerAutoscalingPolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.workerAutoscaling.Policy())
	case http.MethodPost:
		var req control.WorkerAutoscalingPolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.workerAutoscaling.SetPolicy(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "control.autoscaling.policy.updated",
			Message: "worker autoscaling policy updated",
			Fields: map[string]any{
				"enabled":                item.Enabled,
				"min_workers":            item.MinWorkers,
				"max_workers":            item.MaxWorkers,
				"queue_depth_per_worker": item.QueueDepthPerWorker,
				"target_p95_latency_ms":  item.TargetP95LatencyMs,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWorkerAutoscalingRecommend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.WorkerAutoscalingInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	decision := s.workerAutoscaling.Recommend(req)
	writeJSON(w, http.StatusOK, decision)
}
