package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleMigrationTranslate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.MigrationTranslateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	item, err := s.migrationTooling.Translate(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleMigrationTranslations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.migrationTooling.ListTranslations())
}

func (s *Server) handleMigrationTranslationAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/migrations/translations/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "migrations" || parts[2] != "translations" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.migrationTooling.GetTranslation(parts[3])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "translation not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleMigrationEquivalence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.MigrationEquivalenceInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	result, err := s.migrationTooling.Equivalence(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	code := http.StatusOK
	if !result.Pass {
		code = http.StatusConflict
	}
	writeJSON(w, code, result)
}

func (s *Server) handleMigrationDiffReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TranslationID string `json:"translation_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	result, err := s.migrationTooling.DiffReport(req.TranslationID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMigrationDeprecationScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.MigrationDeprecationScanInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	result, err := s.migrationTooling.DeprecationScan(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	code := http.StatusOK
	if result.UrgencyScore >= 80 {
		code = http.StatusConflict
	}
	writeJSON(w, code, result)
}
