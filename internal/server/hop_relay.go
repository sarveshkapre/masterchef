package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleRelayEndpoints(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.hopRelay.ListEndpoints())
	case http.MethodPost:
		var req control.HopRelayEndpointInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.hopRelay.UpsertEndpoint(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "execution.relay.endpoint.upserted",
			Message: "relay endpoint configured",
			Fields: map[string]any{
				"endpoint_id": item.ID,
				"kind":        item.Kind,
				"region":      item.Region,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRelayEndpointAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/execution/relays/endpoints/{id}
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "execution" || parts[2] != "relays" || parts[3] != "endpoints" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.hopRelay.GetEndpoint(parts[4])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "relay endpoint not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleRelaySessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 100
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		writeJSON(w, http.StatusOK, s.hopRelay.ListSessions(limit))
	case http.MethodPost:
		var req control.HopRelaySessionInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.hopRelay.OpenSession(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "execution.relay.session.opened",
			Message: "egress-only relay session opened",
			Fields: map[string]any{
				"session_id":  item.ID,
				"endpoint_id": item.EndpointID,
				"node_id":     item.NodeID,
				"target_host": item.TargetHost,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
