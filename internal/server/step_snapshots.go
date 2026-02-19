package server

import (
	"encoding/json"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleStepSnapshots(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 100
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		items := s.stepSnapshots.List(control.StepSnapshotQuery{
			RunID:  strings.TrimSpace(r.URL.Query().Get("run_id")),
			JobID:  strings.TrimSpace(r.URL.Query().Get("job_id")),
			StepID: strings.TrimSpace(r.URL.Query().Get("step_id")),
			Limit:  limit,
		})
		writeJSON(w, http.StatusOK, map[string]any{"count": len(items), "items": items})
	case http.MethodPost:
		var req control.StepSnapshotInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.stepSnapshots.Record(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleStepSnapshotByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(path.Base(r.URL.Path))
	if id == "" || id == "snapshots" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "snapshot id is required"})
		return
	}
	item, ok := s.stepSnapshots.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "snapshot not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
