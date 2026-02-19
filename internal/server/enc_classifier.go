package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleENCProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.encProviders.List())
	case http.MethodPost:
		var req control.ENCProviderInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		provider, err := s.encProviders.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "inventory.enc.provider.upserted",
			Message: "enc provider registered",
			Fields: map[string]any{
				"provider_id": provider.ID,
				"name":        provider.Name,
				"enabled":     provider.Enabled,
			},
		}, true)
		writeJSON(w, http.StatusCreated, provider)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleENCProviderAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "inventory" || parts[2] != "node-classifiers" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := strings.TrimSpace(parts[3])
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider id is required"})
		return
	}
	if len(parts) == 4 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, ok := s.encProviders.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "enc provider not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) == 5 {
		action := parts[4]
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch action {
		case "enable", "disable":
			item, err := s.encProviders.SetEnabled(id, action == "enable")
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, item)
			return
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown provider action"})
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

func (s *Server) handleENCClassify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.ENCClassifyInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	result, err := s.encProviders.Classify(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}
