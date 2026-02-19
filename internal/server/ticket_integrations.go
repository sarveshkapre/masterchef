package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleTicketIntegrations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.ticketIntegrations.List())
	case http.MethodPost:
		var req control.TicketIntegrationInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.ticketIntegrations.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTicketIntegrationAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/change-records/ticket-integrations/{id}[/enable|disable]
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "change-records" || parts[2] != "ticket-integrations" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := parts[3]
	if len(parts) == 4 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, ok := s.ticketIntegrations.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "ticket integration not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) == 5 {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var enabled bool
		switch parts[4] {
		case "enable":
			enabled = true
		case "disable":
			enabled = false
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		item, err := s.ticketIntegrations.SetEnabled(id, enabled)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (s *Server) handleTicketSync(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, s.ticketIntegrations.ListLinks())
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.ChangeTicketSyncInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	_, err := s.changeRecords.Get(req.ChangeRecordID)
	result := s.ticketIntegrations.Sync(req, err == nil)
	if !result.Linked {
		writeJSON(w, http.StatusConflict, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
