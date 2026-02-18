package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handlePluginExtensions(w http.ResponseWriter, r *http.Request) {
	type createReq struct {
		Name        string         `json:"name"`
		Type        string         `json:"type"`
		Description string         `json:"description"`
		Entrypoint  string         `json:"entrypoint"`
		Version     string         `json:"version"`
		Config      map[string]any `json:"config"`
		Enabled     bool           `json:"enabled"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.plugins.List(r.URL.Query().Get("type")))
	case http.MethodPost:
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.plugins.Create(control.PluginExtension{
			Name:        req.Name,
			Type:        control.PluginExtensionType(req.Type),
			Description: req.Description,
			Entrypoint:  req.Entrypoint,
			Version:     req.Version,
			Config:      req.Config,
			Enabled:     req.Enabled,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePluginExtensionAction(w http.ResponseWriter, r *http.Request) {
	// /v1/plugins/extensions/{id} or /v1/plugins/extensions/{id}/enable|disable
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid plugin extension path"})
		return
	}
	id := parts[3]
	if len(parts) == 4 {
		switch r.Method {
		case http.MethodGet:
			item, err := s.plugins.Get(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, item)
		case http.MethodDelete:
			if !s.plugins.Delete(id) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "plugin extension not found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	action := parts[4]
	switch action {
	case "enable":
		item, err := s.plugins.SetEnabled(id, true)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	case "disable":
		item, err := s.plugins.SetEnabled(id, false)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown plugin extension action"})
	}
}
