package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/masterchef/masterchef/internal/checker"
	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/planner"
)

func (s *Server) handleGitOpsReconcile(baseDir string) http.HandlerFunc {
	type reconcileReq struct {
		Branch        string `json:"branch"`
		ConfigPath    string `json:"config_path"`
		MaxDriftItems int    `json:"max_drift_items,omitempty"`
		AutoEnqueue   bool   `json:"auto_enqueue,omitempty"`
		Priority      string `json:"priority,omitempty"`
		Force         bool   `json:"force,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req reconcileReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		configPath := strings.TrimSpace(req.ConfigPath)
		if configPath == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path is required"})
			return
		}
		resolved := configPath
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(baseDir, resolved)
		}
		if _, err := os.Stat(resolved); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path not found"})
			return
		}

		cfg, err := config.Load(resolved)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		plan, err := planner.Build(cfg)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		report := checker.Run(plan)
		allowed := true
		reason := ""
		if report.ChangesNeeded == 0 {
			allowed = false
			reason = "no drift detected"
		}
		if req.MaxDriftItems > 0 && report.ChangesNeeded > req.MaxDriftItems {
			allowed = false
			reason = "drift exceeds max_drift_items guardrail"
		}
		response := map[string]any{
			"branch":          strings.TrimSpace(req.Branch),
			"config_path":     configPath,
			"changes_needed":  report.ChangesNeeded,
			"confidence":      report.Confidence,
			"would_reconcile": allowed,
			"reason":          reason,
			"report":          report,
		}
		if allowed && req.AutoEnqueue {
			job, err := s.queue.Enqueue(resolved, "gitops-reconcile:"+strings.TrimSpace(req.Branch)+":"+configPath, req.Force, req.Priority)
			if err != nil {
				response["enqueue_error"] = err.Error()
				writeJSON(w, http.StatusConflict, response)
				return
			}
			response["job_id"] = job.ID
			s.recordEvent(control.Event{
				Type:    "gitops.reconcile.enqueued",
				Message: "drift-aware gitops reconcile enqueued",
				Fields: map[string]any{
					"branch":         strings.TrimSpace(req.Branch),
					"config_path":    configPath,
					"changes_needed": report.ChangesNeeded,
					"job_id":         job.ID,
				},
			}, true)
		} else {
			s.recordEvent(control.Event{
				Type:    "gitops.reconcile.evaluated",
				Message: "drift-aware gitops reconcile evaluated",
				Fields: map[string]any{
					"branch":          strings.TrimSpace(req.Branch),
					"config_path":     configPath,
					"changes_needed":  report.ChangesNeeded,
					"would_reconcile": allowed,
				},
			}, true)
		}
		writeJSON(w, http.StatusOK, response)
	}
}
