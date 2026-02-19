package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleGitOpsFileSyncPipelines(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.fileSync.List())
	case http.MethodPost:
		var req control.FileSyncPipelineInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.fileSync.Create(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "gitops.filesync.created",
			Message: "staging-to-live file sync pipeline created",
			Fields: map[string]any{
				"pipeline_id": item.ID,
				"name":        item.Name,
				"workers":     item.Workers,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGitOpsFileSyncPipelineAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/gitops/filesync/pipelines/{id} or /{id}/run
	if len(parts) < 5 || parts[0] != "v1" || parts[1] != "gitops" || parts[2] != "filesync" || parts[3] != "pipelines" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := strings.TrimSpace(parts[4])
	if id == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if len(parts) == 5 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, ok := s.fileSync.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file sync pipeline not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) != 6 || parts[5] != "run" || r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, err := s.fileSync.Run(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "gitops.filesync.ran",
		Message: "staging-to-live file sync pipeline run completed",
		Fields: map[string]any{
			"pipeline_id":  item.ID,
			"files_synced": item.FilesSynced,
			"bytes_synced": item.BytesSynced,
		},
	}, true)
	writeJSON(w, http.StatusOK, item)
}
