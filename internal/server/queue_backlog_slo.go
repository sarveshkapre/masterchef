package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleQueueBacklogSLOPolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.queueBacklogSLO.Policy())
	case http.MethodPost:
		var req control.QueueBacklogSLOPolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.queueBacklogSLO.SetPolicy(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleQueueBacklogSLOStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	limit := parseIntQuery(r, "limit", 50)
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	latest, ok := s.queueBacklogSLO.Latest()
	if !ok {
		policy := s.queueBacklogSLO.Policy()
		queue := s.queue.ControlStatus()
		warnThreshold := (policy.Threshold * policy.WarningPercent) / 100
		recoveryThreshold := (policy.Threshold * policy.RecoveryPercent) / 100
		state := "normal"
		if queue.Pending >= policy.Threshold {
			state = "saturated"
		} else if queue.Pending >= warnThreshold {
			state = "warning"
		}
		latest = control.QueueBacklogSLOStatus{
			At:                time.Now().UTC(),
			Pending:           queue.Pending,
			Running:           queue.Running,
			PendingHigh:       queue.PendingHigh,
			PendingNormal:     queue.PendingNormal,
			PendingLow:        queue.PendingLow,
			Threshold:         policy.Threshold,
			WarningThreshold:  warnThreshold,
			RecoveryThreshold: recoveryThreshold,
			State:             state,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"policy":           s.queueBacklogSLO.Policy(),
		"latest":           latest,
		"history":          s.queueBacklogSLO.History(limit),
		"saturation":       s.backlogSatActive,
		"predictive_alert": s.backlogWarnActive,
	})
}
