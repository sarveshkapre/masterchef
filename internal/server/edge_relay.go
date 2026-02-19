package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleEdgeRelaySites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.edgeRelay.ListSites())
	case http.MethodPost:
		var req control.EdgeRelaySiteInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		site, err := s.edgeRelay.UpsertSite(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "edge_relay.site.upserted",
			Message: "edge relay site configured",
			Fields: map[string]any{
				"site_id": site.SiteID,
				"region":  site.Region,
				"mode":    site.Mode,
			},
		}, true)
		writeJSON(w, http.StatusOK, site)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEdgeRelaySiteAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/edge-relay/sites/{id}[/heartbeat|deliver]
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "edge-relay" || parts[2] != "sites" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	siteID := parts[3]
	if len(parts) == 4 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		site, ok := s.edgeRelay.GetSite(siteID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "edge relay site not found"})
			return
		}
		writeJSON(w, http.StatusOK, site)
		return
	}
	if len(parts) == 5 && parts[4] == "heartbeat" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		site, err := s.edgeRelay.Heartbeat(siteID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, site)
		return
	}
	if len(parts) == 5 && parts[4] == "deliver" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Limit int `json:"limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		result, err := s.edgeRelay.Deliver(siteID, req.Limit)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (s *Server) handleEdgeRelayMessages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 200
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		siteID := strings.TrimSpace(r.URL.Query().Get("site_id"))
		writeJSON(w, http.StatusOK, s.edgeRelay.ListMessages(siteID, limit))
	case http.MethodPost:
		var req control.EdgeRelayMessageInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.edgeRelay.QueueMessage(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "edge_relay.message.queued",
			Message: "edge relay message queued",
			Fields: map[string]any{
				"message_id": item.ID,
				"site_id":    item.SiteID,
				"direction":  item.Direction,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
