package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleConvergeTriggers(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			limit := 100
			if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
				if v, err := strconv.Atoi(raw); err == nil && v > 0 {
					limit = v
				}
			}
			writeJSON(w, http.StatusOK, s.convergeTriggers.List(limit))
		case http.MethodPost:
			var req struct {
				Source         string         `json:"source"`
				EventType      string         `json:"event_type,omitempty"`
				EventID        string         `json:"event_id,omitempty"`
				ConfigPath     string         `json:"config_path"`
				Priority       string         `json:"priority,omitempty"`
				IdempotencyKey string         `json:"idempotency_key,omitempty"`
				Force          bool           `json:"force,omitempty"`
				AutoEnqueue    *bool          `json:"auto_enqueue,omitempty"`
				Payload        map[string]any `json:"payload,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			autoEnqueue := true
			if req.AutoEnqueue != nil {
				autoEnqueue = *req.AutoEnqueue
			}
			if strings.TrimSpace(req.Source) == "" {
				req.Source = "manual"
			}
			if strings.TrimSpace(req.Priority) == "" {
				req.Priority = "normal"
			}
			trigger, err := s.convergeTriggers.NewTrigger(control.ConvergeTriggerInput{
				Source:         req.Source,
				EventType:      req.EventType,
				EventID:        req.EventID,
				ConfigPath:     normalizeConvergeConfigPath(baseDir, req.ConfigPath),
				Priority:       req.Priority,
				IdempotencyKey: req.IdempotencyKey,
				Force:          req.Force,
				AutoEnqueue:    autoEnqueue,
				Payload:        req.Payload,
			})
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}

			statusCode := http.StatusAccepted
			if trigger.AutoEnqueue {
				job, err := s.queue.Enqueue(trigger.ConfigPath, trigger.IdempotencyKey, trigger.Force, trigger.Priority)
				if err != nil {
					statusCode = http.StatusConflict
					trigger, _ = s.convergeTriggers.UpdateOutcome(trigger.ID, control.ConvergeTriggerBlocked, "", err.Error())
				} else {
					trigger, _ = s.convergeTriggers.UpdateOutcome(trigger.ID, control.ConvergeTriggerQueued, job.ID, "")
				}
			}

			s.recordEvent(control.Event{
				Type:    "converge.triggered",
				Message: "converge trigger recorded",
				Fields: map[string]any{
					"trigger_id":    trigger.ID,
					"source":        trigger.Source,
					"event_type":    trigger.EventType,
					"status":        trigger.Status,
					"job_id":        trigger.JobID,
					"enqueue_error": trigger.EnqueueError,
				},
			}, true)
			writeJSON(w, statusCode, trigger)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func normalizeConvergeConfigPath(baseDir, configPath string) string {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" || filepath.IsAbs(configPath) {
		return configPath
	}
	return filepath.Join(baseDir, configPath)
}

func (s *Server) handleConvergeTriggerByID(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/converge/triggers/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "converge" || parts[2] != "triggers" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.convergeTriggers.Get(parts[3])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "converge trigger not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
