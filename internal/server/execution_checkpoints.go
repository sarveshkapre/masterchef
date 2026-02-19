package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/planner"
)

func (s *Server) handleExecutionCheckpoints(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			limit := 100
			if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
				if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
					limit = parsed
				}
			}
			items := s.checkpoints.List(
				strings.TrimSpace(r.URL.Query().Get("run_id")),
				strings.TrimSpace(r.URL.Query().Get("job_id")),
				limit,
			)
			writeJSON(w, http.StatusOK, map[string]any{"count": len(items), "items": items})
		case http.MethodPost:
			var req control.ExecutionCheckpointInput
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			if strings.TrimSpace(req.ConfigPath) != "" && !filepath.IsAbs(req.ConfigPath) {
				req.ConfigPath = filepath.Join(baseDir, req.ConfigPath)
			}
			item, err := s.checkpoints.Record(req)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, item)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleExecutionCheckpointByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := filepath.Base(r.URL.Path)
	if id == "" || id == "checkpoints" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "checkpoint id is required"})
		return
	}
	item, ok := s.checkpoints.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "checkpoint not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleExecutionCheckpointResume(baseDir string) http.HandlerFunc {
	type reqBody struct {
		CheckpointID   string `json:"checkpoint_id"`
		ConfigPath     string `json:"config_path,omitempty"`
		Priority       string `json:"priority,omitempty"`
		IdempotencyKey string `json:"idempotency_key,omitempty"`
		Force          bool   `json:"force,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		checkpoint, ok := s.checkpoints.Get(strings.TrimSpace(req.CheckpointID))
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "checkpoint not found"})
			return
		}
		configPath := strings.TrimSpace(req.ConfigPath)
		if configPath == "" {
			configPath = checkpoint.ConfigPath
		}
		if configPath == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path is required"})
			return
		}
		if !filepath.IsAbs(configPath) {
			configPath = filepath.Join(baseDir, configPath)
		}
		if _, err := os.Stat(configPath); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("config_path not found: %v", err)})
			return
		}
		resumePath, remaining, err := buildResumeConfigFromCheckpoint(baseDir, configPath, checkpoint)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		key := strings.TrimSpace(req.IdempotencyKey)
		if key == "" {
			key = "resume-" + checkpoint.ID + "-" + time.Now().UTC().Format("20060102T150405")
		}
		job, err := s.queue.Enqueue(resumePath, key, req.Force, req.Priority)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"checkpoint":         checkpoint,
			"resume_config_path": resumePath,
			"remaining_steps":    remaining,
			"job":                job,
		})
	}
}

func buildResumeConfigFromCheckpoint(baseDir, configPath string, checkpoint control.ExecutionCheckpoint) (string, int, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return "", 0, err
	}
	plan, err := planner.Build(cfg)
	if err != nil {
		return "", 0, err
	}
	stepOrder := checkpoint.StepOrder
	if stepOrder <= 0 && strings.TrimSpace(checkpoint.StepID) != "" {
		for _, step := range plan.Steps {
			if step.Resource.ID == checkpoint.StepID {
				stepOrder = step.Order
				break
			}
		}
	}
	if stepOrder < 0 {
		stepOrder = 0
	}
	includeIDs := map[string]struct{}{}
	for _, step := range plan.Steps {
		if step.Order > stepOrder {
			includeIDs[step.Resource.ID] = struct{}{}
		}
	}
	if len(includeIDs) == 0 {
		return "", 0, fmt.Errorf("checkpoint already at end of plan; no remaining steps to resume")
	}
	resources := make([]config.Resource, 0, len(includeIDs))
	for _, res := range cfg.Resources {
		if _, ok := includeIDs[res.ID]; !ok {
			continue
		}
		deps := make([]string, 0, len(res.DependsOn))
		for _, dep := range res.DependsOn {
			if _, ok := includeIDs[dep]; ok {
				deps = append(deps, dep)
			}
		}
		res.DependsOn = deps
		resources = append(resources, res)
	}
	resume := *cfg
	resume.Resources = resources
	encoded, err := json.MarshalIndent(resume, "", "  ")
	if err != nil {
		return "", 0, err
	}
	dir := filepath.Join(baseDir, ".masterchef", "resume")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, err
	}
	out := filepath.Join(dir, checkpoint.ID+"-resume.json")
	if err := os.WriteFile(out, encoded, 0o644); err != nil {
		return "", 0, err
	}
	return out, len(resources), nil
}
