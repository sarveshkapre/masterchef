package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleGitOpsPreviews(baseDir string) http.HandlerFunc {
	type createReq struct {
		Branch         string `json:"branch"`
		Environment    string `json:"environment,omitempty"`
		ConfigPath     string `json:"config_path"`
		ArtifactDigest string `json:"artifact_digest,omitempty"`
		TTLSeconds     int    `json:"ttl_seconds,omitempty"`
		Priority       string `json:"priority,omitempty"`
		Force          bool   `json:"force,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			includeExpired := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_expired")), "true")
			writeJSON(w, http.StatusOK, s.gitopsPreviews.List(includeExpired))
		case http.MethodPost:
			var req createReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			configPath := strings.TrimSpace(req.ConfigPath)
			if configPath == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path is required"})
				return
			}
			resolvedConfig := configPath
			if !filepath.IsAbs(resolvedConfig) {
				resolvedConfig = filepath.Join(baseDir, resolvedConfig)
			}
			if _, err := os.Stat(resolvedConfig); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path not found"})
				return
			}

			preview, err := s.gitopsPreviews.Create(control.GitOpsPreviewInput{
				Branch:         req.Branch,
				Environment:    req.Environment,
				ConfigPath:     configPath,
				ArtifactDigest: req.ArtifactDigest,
				TTLSeconds:     req.TTLSeconds,
			})
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			job, err := s.queue.Enqueue(resolvedConfig, preview.ID, req.Force, req.Priority)
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error":   err.Error(),
					"preview": preview,
				})
				return
			}
			preview, _ = s.gitopsPreviews.AttachJob(preview.ID, job.ID)
			s.recordEvent(control.Event{
				Type:    "gitops.preview.created",
				Message: "branch-based ephemeral environment preview created",
				Fields: map[string]any{
					"preview_id":  preview.ID,
					"branch":      preview.Branch,
					"environment": preview.Environment,
					"job_id":      preview.LastJobID,
				},
			}, true)
			writeJSON(w, http.StatusCreated, preview)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleGitOpsPreviewAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/gitops/previews/{id} or /{id}/promote|close
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "gitops" || parts[2] != "previews" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := strings.TrimSpace(parts[3])
	if id == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if len(parts) == 4 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		preview, ok := s.gitopsPreviews.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "preview not found"})
			return
		}
		writeJSON(w, http.StatusOK, preview)
		return
	}
	if len(parts) != 5 || r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	status := ""
	switch parts[4] {
	case "promote":
		status = control.PreviewStatusPromoted
	case "close":
		status = control.PreviewStatusClosed
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
		return
	}
	preview, err := s.gitopsPreviews.SetStatus(id, status)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "gitops.preview.status",
		Message: "preview status changed",
		Fields: map[string]any{
			"preview_id": preview.ID,
			"status":     preview.Status,
		},
	}, true)
	writeJSON(w, http.StatusOK, preview)
}
