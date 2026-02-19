package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleMultiMasterNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.multiMaster.ListNodes())
	case http.MethodPost:
		var req control.MultiMasterNodeInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.multiMaster.UpsertNode(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "control.multi_master.node.upserted",
			Message: "multi-master control-plane node upserted",
			Fields: map[string]any{
				"node_id": item.NodeID,
				"region":  item.Region,
				"role":    item.Role,
				"status":  item.Status,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMultiMasterNodeAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/control/multi-master/nodes/{id}[/heartbeat]
	if len(parts) < 5 || parts[0] != "v1" || parts[1] != "control" || parts[2] != "multi-master" || parts[3] != "nodes" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	nodeID := parts[4]
	if len(parts) == 5 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, ok := s.multiMaster.GetNode(nodeID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "multi-master node not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) == 6 && parts[5] == "heartbeat" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.multiMaster.Heartbeat(nodeID, req.Status)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (s *Server) handleMultiMasterCache(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 200
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		kind := strings.TrimSpace(r.URL.Query().Get("kind"))
		writeJSON(w, http.StatusOK, s.multiMaster.ListCentralCache(kind, limit))
	case http.MethodPost:
		var req struct {
			EventLimit int `json:"event_limit"`
			MaxEntries int `json:"max_entries"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if req.EventLimit <= 0 {
			req.EventLimit = 500
		}
		events := s.events.Query(control.EventQuery{Limit: req.EventLimit, Desc: true})
		jobs := s.queue.List()
		result := s.multiMaster.SyncCentralCache(jobs, events, req.MaxEntries)
		s.recordEvent(control.Event{
			Type:    "control.multi_master.cache.synced",
			Message: "multi-master centralized cache synchronized",
			Fields: map[string]any{
				"synced_jobs":   result.SyncedJobs,
				"synced_events": result.SyncedEvents,
				"total_entries": result.TotalEntries,
			},
		}, true)
		writeJSON(w, http.StatusOK, result)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
