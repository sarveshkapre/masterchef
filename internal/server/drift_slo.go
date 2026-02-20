package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/state"
)

func (s *Server) handleDriftSLOPolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.driftSLO.Policy())
	case http.MethodPost:
		var req control.DriftSLOPolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.driftSLO.SetPolicy(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDriftSLOEvaluations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	writeJSON(w, http.StatusOK, s.driftSLO.List(limit))
}

func (s *Server) handleDriftSLOEvaluate(baseDir string) http.HandlerFunc {
	type reqBody struct {
		WindowHours        int  `json:"window_hours,omitempty"`
		IncludeSuppressed  bool `json:"include_suppressed,omitempty"`
		IncludeAllowlisted bool `json:"include_allowlisted,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		policy := s.driftSLO.Policy()
		window := req.WindowHours
		if window <= 0 {
			window = policy.WindowHours
		}
		if window > 24*30 {
			window = 24 * 30
		}
		since := time.Now().UTC().Add(-time.Duration(window) * time.Hour)

		runs, err := state.New(baseDir).ListRuns(5000)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		samples := 0
		changed := 0
		suppressed := 0
		allowlisted := 0
		failedRuns := 0
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
				if res.Skipped {
					continue
				}
				samples++
				if !res.Changed {
					continue
				}
				if s.driftPolicies != nil && s.driftPolicies.IsSuppressed(res.Host, res.Type, res.ResourceID, ref) {
					suppressed++
					if !req.IncludeSuppressed {
						continue
					}
				}
				if s.driftPolicies != nil && s.driftPolicies.IsAllowlisted(res.Host, res.Type, res.ResourceID, ref) {
					allowlisted++
					if !req.IncludeAllowlisted {
						continue
					}
				}
				changed++
			}
		}

		eval := s.driftSLO.Evaluate(control.DriftSLOEvaluationInput{
			WindowHours:        window,
			Samples:            samples,
			Changed:            changed,
			Suppressed:         suppressed,
			Allowlisted:        allowlisted,
			FailedRuns:         failedRuns,
			IncludeSuppressed:  req.IncludeSuppressed,
			IncludeAllowlisted: req.IncludeAllowlisted,
		})
		if eval.IncidentRecommended {
			s.recordEvent(control.Event{
				Type:    "drift.slo.incident.triggered",
				Message: "drift SLO breach detected; incident hook triggered",
				Fields: map[string]any{
					"evaluation_id":      eval.ID,
					"window_hours":       eval.WindowHours,
					"samples":            eval.Samples,
					"changed":            eval.Changed,
					"compliance_percent": eval.CompliancePercent,
					"target_percent":     eval.TargetPercent,
					"incident_hook":      eval.IncidentHook,
				},
			}, true)
		}
		code := http.StatusOK
		if eval.Breached {
			code = http.StatusConflict
		}
		writeJSON(w, code, eval)
	}
}
