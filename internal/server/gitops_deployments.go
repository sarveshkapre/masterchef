package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleGitOpsDeploymentTriggerAlias(baseDir, source string) http.HandlerFunc {
	h := s.handleGitOpsDeployments(baseDir)
	source = strings.ToLower(strings.TrimSpace(source))
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		r.Header.Set("X-Deployment-Source", source)
		h(w, r)
	}
}

func (s *Server) handleGitOpsDeployments(baseDir string) http.HandlerFunc {
	type createReq struct {
		Environment string `json:"environment"`
		Branch      string `json:"branch"`
		ConfigPath  string `json:"config_path"`
		Source      string `json:"source,omitempty"`
		Priority    string `json:"priority,omitempty"`
		Force       bool   `json:"force,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.deployments.List())
		case http.MethodPost:
			var req createReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			env := strings.ToLower(strings.TrimSpace(req.Environment))
			branch := strings.TrimSpace(req.Branch)
			configPath := strings.TrimSpace(req.ConfigPath)
			if env == "" || branch == "" || configPath == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment, branch, and config_path are required"})
				return
			}
			source := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Deployment-Source")))
			if source == "" {
				source = strings.ToLower(strings.TrimSpace(req.Source))
			}
			if source == "" {
				source = "api"
			}
			resolved := configPath
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(baseDir, resolved)
			}
			if _, err := os.Stat(resolved); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path not found"})
				return
			}
			key := "gitops-deploy:" + env + ":" + branch + ":" + configPath
			job, err := s.queue.Enqueue(resolved, key, req.Force, req.Priority)
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			item, err := s.deployments.Create(control.DeploymentTriggerInput{
				Environment: env,
				Branch:      branch,
				ConfigPath:  configPath,
				Source:      source,
				Priority:    req.Priority,
				Force:       req.Force,
				JobID:       job.ID,
			})
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			s.recordEvent(control.Event{
				Type:    "gitops.deployment.triggered",
				Message: "environment code deployment triggered",
				Fields: map[string]any{
					"id":          item.ID,
					"environment": item.Environment,
					"branch":      item.Branch,
					"source":      item.Source,
					"job_id":      item.JobID,
				},
			}, true)
			writeJSON(w, http.StatusCreated, item)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleGitOpsDeploymentAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/gitops/deployments/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "gitops" || parts[2] != "deployments" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.deployments.Get(parts[3])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "deployment trigger not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
