package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleCanaryUpgrades(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Component    string   `json:"component"`
		ToChannel    string   `json:"to_channel"`
		AutoRollback bool     `json:"auto_rollback"`
		CanaryIDs    []string `json:"canary_ids"`
	}
	switch r.Method {
	case http.MethodGet:
		limit := 100
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		writeJSON(w, http.StatusOK, s.canaryUpgrades.List(limit))
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		component := strings.ToLower(strings.TrimSpace(req.Component))
		if component == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "component is required"})
			return
		}
		toChannel := strings.ToLower(strings.TrimSpace(req.ToChannel))
		if toChannel == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to_channel is required"})
			return
		}
		fromChannel := "stable"
		for _, item := range s.channels.List() {
			if item.Component == component {
				fromChannel = item.Channel
				break
			}
		}
		if _, err := s.channels.SetChannel(component, toChannel); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		regression := false
		reason := "canary upgrade healthy"
		if len(req.CanaryIDs) > 0 {
			for _, id := range req.CanaryIDs {
				canary, err := s.canaries.Get(id)
				if err != nil {
					regression = true
					reason = "canary not found: " + strings.TrimSpace(id)
					break
				}
				if canary.Health != control.CanaryHealthy {
					regression = true
					reason = "canary regression detected: " + strings.TrimSpace(id)
					break
				}
			}
		} else {
			summary := s.canaries.HealthSummary()
			if unhealthy, ok := summary["unhealthy"].(int); ok && unhealthy > 0 {
				regression = true
				reason = "canary health summary reports unhealthy checks"
			}
		}

		status := "completed"
		rolledBack := false
		if regression && req.AutoRollback {
			_, _ = s.channels.SetChannel(component, fromChannel)
			status = "rolled_back"
			rolledBack = true
		} else if regression {
			status = "regression_detected"
		}

		run, err := s.canaryUpgrades.Record(control.CanaryUpgradeRun{
			Component:    component,
			FromChannel:  fromChannel,
			ToChannel:    toChannel,
			CanaryIDs:    req.CanaryIDs,
			AutoRollback: req.AutoRollback,
			Status:       status,
			RolledBack:   rolledBack,
			Reason:       reason,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "control.upgrade.canary",
			Message: "control-plane canary upgrade evaluated",
			Fields: map[string]any{
				"run_id":       run.ID,
				"component":    run.Component,
				"from_channel": run.FromChannel,
				"to_channel":   run.ToChannel,
				"status":       run.Status,
				"rolled_back":  run.RolledBack,
			},
		}, true)
		code := http.StatusOK
		if run.Status == "rolled_back" || run.Status == "regression_detected" {
			code = http.StatusConflict
		}
		writeJSON(w, code, run)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCanaryUpgradeAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/control/canary-upgrades/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "control" || parts[2] != "canary-upgrades" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.canaryUpgrades.Get(parts[3])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "canary upgrade run not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
