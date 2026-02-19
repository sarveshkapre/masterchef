package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleSchedulerPartitions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.schedulerPartitions.List())
	case http.MethodPost:
		var req control.SchedulerPartitionRuleInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.schedulerPartitions.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "control.scheduler.partition.rule",
			Message: "scheduler partition rule updated",
			Fields: map[string]any{
				"rule_id":      item.ID,
				"tenant":       item.Tenant,
				"environment":  item.Environment,
				"region":       item.Region,
				"shard":        item.Shard,
				"max_parallel": item.MaxParallel,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSchedulerPartitionAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/control/scheduler/partitions/{id}
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "control" || parts[2] != "scheduler" || parts[3] != "partitions" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.schedulerPartitions.Get(parts[4])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "partition rule not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleSchedulerPartitionDecision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.SchedulerPartitionDecisionInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	decision := s.schedulerPartitions.Decide(req)
	writeJSON(w, http.StatusOK, decision)
}
