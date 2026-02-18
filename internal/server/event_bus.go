package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleEventBusTargets(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Name    string            `json:"name"`
		Kind    string            `json:"kind"`
		URL     string            `json:"url"`
		Topic   string            `json:"topic"`
		Headers map[string]string `json:"headers"`
		Enabled bool              `json:"enabled"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.eventBus.ListTargets())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		target, err := s.eventBus.Register(control.EventBusTarget{
			Name:    req.Name,
			Kind:    control.EventBusKind(req.Kind),
			URL:     req.URL,
			Topic:   req.Topic,
			Headers: req.Headers,
			Enabled: req.Enabled,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, target)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEventBusTargetAction(w http.ResponseWriter, r *http.Request) {
	// /v1/event-bus/targets/{id}/enable|disable
	parts := splitPath(r.URL.Path)
	if len(parts) < 5 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid event-bus target path"})
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := parts[3]
	action := parts[4]
	switch action {
	case "enable":
		item, err := s.eventBus.SetEnabled(id, true)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	case "disable":
		item, err := s.eventBus.SetEnabled(id, false)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown event-bus target action"})
	}
}

func (s *Server) handleEventBusDeliveries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	writeJSON(w, http.StatusOK, s.eventBus.Deliveries(limit))
}

func (s *Server) handleEventBusPublish(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Type    string         `json:"type"`
		Message string         `json:"message"`
		Fields  map[string]any `json:"fields"`
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req reqBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if strings.TrimSpace(req.Type) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type is required"})
		return
	}
	evt := control.Event{
		Type:    req.Type,
		Message: req.Message,
		Fields:  req.Fields,
	}
	d := s.eventBus.Publish(evt)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":     "published",
		"deliveries": d,
	})
}
