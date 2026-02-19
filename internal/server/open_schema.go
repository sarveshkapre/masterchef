package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleOpenSchemas(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.openSchemas.List())
	case http.MethodPost:
		var req control.OpenSchemaInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		doc, err := s.openSchemas.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, doc)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOpenSchemaByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := filepath.Base(r.URL.Path)
	if id == "" || id == "models" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "schema id is required"})
		return
	}
	doc, ok := s.openSchemas.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schema not found"})
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *Server) handleOpenSchemaValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.OpenSchemaValidationInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	result := s.openSchemas.Validate(req)
	status := http.StatusOK
	if !result.Valid {
		status = http.StatusConflict
	}
	writeJSON(w, status, result)
}
