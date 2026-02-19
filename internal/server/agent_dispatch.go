package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleAgentDispatchMode(w http.ResponseWriter, r *http.Request) {
	type modeReq struct {
		Mode string `json:"mode"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]string{"mode": s.agentDispatch.Mode()})
	case http.MethodPost:
		var req modeReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		mode, err := s.agentDispatch.SetMode(req.Mode)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "agent.dispatch.mode",
			Message: "agent dispatch mode changed",
			Fields:  map[string]any{"mode": mode},
		}, true)
		writeJSON(w, http.StatusOK, map[string]string{"mode": mode})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAgentDispatch(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			limit := 100
			if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n > 0 {
					limit = n
				}
			}
			writeJSON(w, http.StatusOK, s.agentDispatch.List(limit))
		case http.MethodPost:
			var req control.AgentDispatchRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			req.ConfigPath = strings.TrimSpace(req.ConfigPath)
			if req.ConfigPath == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path is required"})
				return
			}
			mode := s.agentDispatch.Mode()
			switch mode {
			case control.AgentDispatchModeLocal:
				resolved := req.ConfigPath
				if !filepath.IsAbs(resolved) {
					resolved = filepath.Join(baseDir, resolved)
				}
				if _, err := os.Stat(resolved); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path not found"})
					return
				}
				job, err := s.queue.Enqueue(resolved, "agent-dispatch:"+req.ConfigPath, req.Force, req.Priority)
				if err != nil {
					writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
					return
				}
				item := s.agentDispatch.Record(mode, req, "queued", job.ID)
				s.recordEvent(control.Event{
					Type:    "agent.dispatch.queued",
					Message: "agent dispatch queued locally",
					Fields: map[string]any{
						"dispatch_id": item.ID,
						"job_id":      item.JobID,
						"config_path": item.ConfigPath,
					},
				}, true)
				writeJSON(w, http.StatusCreated, item)
			case control.AgentDispatchModeEventBus:
				event := control.Event{
					Type:    "agent.dispatch.request",
					Message: "agent dispatch requested over event bus",
					Fields: map[string]any{
						"config_path": req.ConfigPath,
						"priority":    strings.ToLower(strings.TrimSpace(req.Priority)),
						"force":       req.Force,
					},
				}
				_ = s.eventBus.Publish(event)
				item := s.agentDispatch.Record(mode, req, "dispatched", "")
				s.recordEvent(control.Event{
					Type:    "agent.dispatch.dispatched",
					Message: "agent dispatch published to event bus",
					Fields: map[string]any{
						"dispatch_id": item.ID,
						"config_path": item.ConfigPath,
					},
				}, true)
				writeJSON(w, http.StatusCreated, item)
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported dispatch mode"})
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}
