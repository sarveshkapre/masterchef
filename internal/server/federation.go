package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleFederationPeers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.federation.ListPeers())
	case http.MethodPost:
		var req control.FederationPeerInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.federation.UpsertPeer(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "control.federation.peer.upserted",
			Message: "federation peer configured",
			Fields: map[string]any{
				"peer_id":  item.ID,
				"region":   item.Region,
				"mode":     item.Mode,
				"endpoint": item.Endpoint,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleFederationPeerAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/control/federation/peers/{id}/health
	if len(parts) < 5 || parts[0] != "v1" || parts[1] != "control" || parts[2] != "federation" || parts[3] != "peers" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if len(parts) == 5 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, ok := s.federation.GetPeer(parts[4])
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "federation peer not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) == 6 && parts[5] == "health" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Healthy   bool `json:"healthy"`
			LatencyMs int  `json:"latency_ms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.federation.SetPeerHealth(parts[4], req.Healthy, req.LatencyMs)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (s *Server) handleFederationHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.federation.HealthMatrix())
}
