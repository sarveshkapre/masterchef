package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleRuntimeEnrollAlias(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.handleRuntimeHosts(w, r)
}

func (s *Server) handleRuntimeHosts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status := r.URL.Query().Get("status")
		writeJSON(w, http.StatusOK, s.nodes.List(status))
	case http.MethodPost:
		var req control.NodeEnrollInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		node, created, err := s.nodes.Enroll(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		eventType := "inventory.node.updated"
		if created {
			eventType = "inventory.node.enrolled"
		}
		s.recordEvent(control.Event{
			Type:    eventType,
			Message: "runtime node metadata upserted",
			Fields: map[string]any{
				"name":      node.Name,
				"status":    node.Status,
				"transport": node.Transport,
				"source":    node.Source,
			},
		}, true)
		code := http.StatusOK
		if created {
			code = http.StatusCreated
		}
		writeJSON(w, code, node)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRuntimeHostAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/inventory/runtime-hosts/{name} or /{name}/{action}
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "inventory" || parts[2] != "runtime-hosts" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	name := strings.TrimSpace(parts[3])
	if name == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if len(parts) == 4 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		node, ok := s.nodes.Get(name)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "runtime host not found"})
			return
		}
		writeJSON(w, http.StatusOK, node)
		return
	}
	if len(parts) != 5 || r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	action := parts[4]
	type actionReq struct {
		Reason string `json:"reason,omitempty"`
	}
	var req actionReq
	_ = json.NewDecoder(r.Body).Decode(&req)

	switch action {
	case "heartbeat":
		node, err := s.nodes.Heartbeat(name)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "inventory.node.heartbeat",
			Message: "runtime node heartbeat received",
			Fields: map[string]any{
				"name":   node.Name,
				"status": node.Status,
			},
		}, true)
		writeJSON(w, http.StatusOK, node)
		return
	case "bootstrap":
		s.updateRuntimeHostStatus(w, name, control.NodeStatusBootstrap, req.Reason)
	case "activate":
		s.updateRuntimeHostStatus(w, name, control.NodeStatusActive, req.Reason)
	case "quarantine":
		s.updateRuntimeHostStatus(w, name, control.NodeStatusQuarantined, req.Reason)
	case "decommission":
		s.updateRuntimeHostStatus(w, name, control.NodeStatusDecommissioned, req.Reason)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
	}
}

func (s *Server) updateRuntimeHostStatus(w http.ResponseWriter, name, status, reason string) {
	node, err := s.nodes.SetStatus(name, status, reason)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "inventory.node.status_changed",
		Message: "runtime node lifecycle status updated",
		Fields: map[string]any{
			"name":      node.Name,
			"status":    node.Status,
			"reason":    strings.TrimSpace(reason),
			"transport": node.Transport,
		},
	}, true)
	writeJSON(w, http.StatusOK, node)
}
