package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/state"
)

func (s *Server) handleDriftRemediation(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Hours       int    `json:"hours,omitempty"`
			ConfigPath  string `json:"config_path"`
			Priority    string `json:"priority,omitempty"`
			Force       bool   `json:"force,omitempty"`
			SafeMode    bool   `json:"safe_mode,omitempty"`
			MaxChanges  int    `json:"max_changes,omitempty"`
			AutoEnqueue bool   `json:"auto_enqueue,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if req.Hours <= 0 {
			req.Hours = 24
		}
		if req.Hours > 24*30 {
			req.Hours = 24 * 30
		}
		if req.MaxChanges <= 0 {
			req.MaxChanges = 20
		}
		if strings.TrimSpace(req.Priority) == "" {
			req.Priority = "normal"
		}
		if strings.TrimSpace(req.ConfigPath) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path is required"})
			return
		}
		configPath := normalizeConvergeConfigPath(baseDir, req.ConfigPath)
		since := time.Now().UTC().Add(-time.Duration(req.Hours) * time.Hour)

		runs, err := state.New(baseDir).ListRuns(5000)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		candidates := map[string]struct{}{}
		failedRuns := 0
		suppressed := 0
		allowlisted := 0
		for _, run := range runs {
			ref := run.StartedAt
			if ref.IsZero() {
				ref = run.EndedAt
			}
			if ref.IsZero() || ref.Before(since) {
				continue
			}
			if run.Status == state.RunFailed {
				failedRuns++
			}
			for _, res := range run.Results {
				if !res.Changed {
					continue
				}
				if s.driftPolicies != nil && s.driftPolicies.IsSuppressed(res.Host, res.Type, res.ResourceID, ref) {
					suppressed++
					continue
				}
				if s.driftPolicies != nil && s.driftPolicies.IsAllowlisted(res.Host, res.Type, res.ResourceID, ref) {
					allowlisted++
					continue
				}
				key := strings.ToLower(strings.TrimSpace(res.Host)) + "|" + strings.ToLower(strings.TrimSpace(res.Type)) + "|" + strings.ToLower(strings.TrimSpace(res.ResourceID))
				candidates[key] = struct{}{}
			}
		}

		candidateCount := len(candidates)
		riskLevel := "low"
		blockReasons := make([]string, 0, 2)
		if candidateCount > req.MaxChanges {
			riskLevel = "high"
			blockReasons = append(blockReasons, "candidate change count exceeds safe threshold")
		}
		if failedRuns > 0 {
			riskLevel = "high"
			blockReasons = append(blockReasons, "recent failed runs increase remediation risk")
		}

		response := map[string]any{
			"window_hours":        req.Hours,
			"since":               since,
			"candidate_changes":   candidateCount,
			"suppressed_changes":  suppressed,
			"allowlisted_changes": allowlisted,
			"failed_runs":         failedRuns,
			"safe_mode":           req.SafeMode,
			"max_changes":         req.MaxChanges,
			"risk_level":          riskLevel,
			"block_reasons":       blockReasons,
		}
		if candidateCount == 0 {
			response["status"] = "noop"
			writeJSON(w, http.StatusOK, response)
			return
		}
		if req.SafeMode && len(blockReasons) > 0 {
			response["status"] = "blocked"
			writeJSON(w, http.StatusConflict, response)
			return
		}

		job, err := s.queue.Enqueue(configPath, "drift-remediate:"+strconv.FormatInt(time.Now().UTC().UnixNano(), 10), req.Force, req.Priority)
		if err != nil {
			response["status"] = "blocked"
			response["enqueue_error"] = err.Error()
			writeJSON(w, http.StatusConflict, response)
			return
		}
		response["status"] = "enqueued"
		response["job_id"] = job.ID
		response["config_path"] = configPath
		writeJSON(w, http.StatusAccepted, response)
	}
}
