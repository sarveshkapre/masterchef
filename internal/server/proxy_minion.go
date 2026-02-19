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

func (s *Server) handleProxyMinions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.proxyMinions.ListBindings())
	case http.MethodPost:
		var req control.ProxyMinionBindingInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.proxyMinions.UpsertBinding(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "agents.proxy_minion.binding.upserted",
			Message: "proxy-minion binding registered",
			Fields: map[string]any{
				"binding_id": item.ID,
				"proxy_id":   item.ProxyID,
				"device_id":  item.DeviceID,
				"transport":  item.Transport,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProxyMinionAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/agents/proxy-minions/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "agents" || parts[2] != "proxy-minions" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.proxyMinions.GetBinding(parts[3])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "proxy-minion binding not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleProxyMinionDispatch(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			limit := 100
			if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n > 0 {
					limit = n
				}
			}
			writeJSON(w, http.StatusOK, s.proxyMinions.ListDispatches(limit))
		case http.MethodPost:
			var req control.ProxyMinionDispatchRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			deviceID := strings.TrimSpace(req.DeviceID)
			if deviceID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device_id is required"})
				return
			}
			binding, ok := s.proxyMinions.ResolveDevice(deviceID)
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "no proxy-minion binding for device"})
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
			job, err := s.queue.Enqueue(resolved, "proxy-minion:"+binding.ProxyID+":"+binding.DeviceID+":"+configPath, req.Force, req.Priority)
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			rec := s.proxyMinions.RecordDispatch(binding, req, "queued", job.ID)
			s.recordEvent(control.Event{
				Type:    "agents.proxy_minion.dispatched",
				Message: "proxy-minion dispatch queued",
				Fields: map[string]any{
					"dispatch_id": rec.ID,
					"device_id":   rec.DeviceID,
					"proxy_id":    rec.ProxyID,
					"job_id":      rec.JobID,
				},
			}, true)
			writeJSON(w, http.StatusCreated, rec)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}
