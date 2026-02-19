package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleDiscoverySources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.discoveryInventory.ListSources())
	case http.MethodPost:
		var req control.DiscoverySourceInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.discoveryInventory.CreateSource(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "inventory.discovery.source.created",
			Message: "inventory discovery source registered",
			Fields: map[string]any{
				"source_id": item.ID,
				"kind":      item.Kind,
				"enabled":   item.Enabled,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDiscoverySourceAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/inventory/discovery-sources/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "inventory" || parts[2] != "discovery-sources" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.discoveryInventory.GetSource(parts[3])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "discovery source not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleDiscoverySourceSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.DiscoverySyncInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	source, enrolls, report, err := s.discoveryInventory.PrepareSync(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	created := 0
	updated := 0
	for _, in := range enrolls {
		_, wasCreated, err := s.nodes.Enroll(in)
		if err != nil {
			continue
		}
		if wasCreated {
			created++
		} else {
			updated++
		}
	}
	response := map[string]any{
		"source_id":       report.SourceID,
		"kind":            report.Kind,
		"requested_hosts": report.RequestedHosts,
		"valid_hosts":     report.ValidHosts,
		"created":         created,
		"updated":         updated,
	}
	s.recordEvent(control.Event{
		Type:    "inventory.discovery.sync",
		Message: "inventory discovery sync applied",
		Fields: map[string]any{
			"source_id": source.ID,
			"kind":      source.Kind,
			"created":   created,
			"updated":   updated,
		},
	}, true)
	writeJSON(w, http.StatusOK, response)
}
