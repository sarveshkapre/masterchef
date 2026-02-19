package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleRunLeases(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		includeRecovered := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_recovered")), "true")
		writeJSON(w, http.StatusOK, s.runLeases.List(includeRecovered))
	case http.MethodPost:
		var req control.RunLeaseAcquireInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		lease, err := s.runLeases.Acquire(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.events.Append(control.Event{
			Type:    "control.run_lease.acquired",
			Message: "run lease acquired",
			Fields: map[string]any{
				"lease_id": lease.LeaseID,
				"job_id":   lease.JobID,
				"holder":   lease.Holder,
				"ttl":      lease.TTLSeconds,
			},
		})
		writeJSON(w, http.StatusCreated, lease)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRunLeaseHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.RunLeaseHeartbeatInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	lease, err := s.runLeases.Heartbeat(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, lease)
}

func (s *Server) handleRunLeaseRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.RunLeaseHeartbeatInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	lease, err := s.runLeases.Release(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, lease)
}

func (s *Server) handleRunLeaseRecover(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Now string `json:"now,omitempty"`
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req reqBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	now := time.Now().UTC()
	if strings.TrimSpace(req.Now) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(req.Now))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "now must be RFC3339"})
			return
		}
		now = parsed.UTC()
	}
	recovered := s.runLeases.RecoverExpired(now)
	failedJobs := make([]control.Job, 0, len(recovered))
	for _, lease := range recovered {
		job, err := s.queue.FailJob(lease.JobID, "stale run lease recovered by control plane")
		if err == nil {
			failedJobs = append(failedJobs, job)
		}
	}
	if len(recovered) > 0 {
		s.events.Append(control.Event{
			Type:    "control.run_lease.recovered",
			Message: "stale run leases recovered",
			Fields: map[string]any{
				"count": len(recovered),
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"recovered_count": len(recovered),
		"recovered":       recovered,
		"failed_jobs":     failedJobs,
	})
}
