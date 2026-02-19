package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleSyndicNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.syndic.List())
	case http.MethodPost:
		var req control.SyndicNodeInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.syndic.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "control.syndic.node.upserted",
			Message: "syndic topology node updated",
			Fields: map[string]any{
				"node_id": item.ID,
				"name":    item.Name,
				"role":    item.Role,
				"parent":  item.Parent,
				"region":  item.Region,
				"segment": item.Segment,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSyndicRoute(w http.ResponseWriter, r *http.Request) {
	resolve := func(target string) {
		route, err := s.syndic.ResolveRoute(target)
		if err == nil {
			writeJSON(w, http.StatusOK, route)
			return
		}
		msg := err.Error()
		switch {
		case msg == "target node not found":
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg})
		case strings.Contains(msg, "topology"):
			writeJSON(w, http.StatusConflict, map[string]string{"error": msg})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		}
	}

	switch r.Method {
	case http.MethodGet:
		target := strings.TrimSpace(r.URL.Query().Get("target"))
		if target == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target is required"})
			return
		}
		resolve(target)
	case http.MethodPost:
		var req control.SyndicRouteInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		if strings.TrimSpace(req.Target) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target is required"})
			return
		}
		resolve(req.Target)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
