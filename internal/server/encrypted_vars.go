package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) handleEncryptedVariableKeys(w http.ResponseWriter, r *http.Request) {
	type rotateReq struct {
		OldPassphrase string `json:"old_passphrase"`
		NewPassphrase string `json:"new_passphrase"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.encryptedVars.KeyStatus())
	case http.MethodPost:
		var req rotateReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		result, err := s.encryptedVars.Rotate(req.OldPassphrase, req.NewPassphrase)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEncryptedVariableFiles(w http.ResponseWriter, r *http.Request) {
	type upsertReq struct {
		Name       string         `json:"name"`
		Data       map[string]any `json:"data"`
		Passphrase string         `json:"passphrase"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"items": s.encryptedVars.List(),
			"keys":  s.encryptedVars.KeyStatus(),
		})
	case http.MethodPost:
		var req upsertReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.encryptedVars.Upsert(req.Name, req.Data, req.Passphrase)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEncryptedVariableFileAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) < 5 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid encrypted variable file path"})
		return
	}
	name := parts[4]
	switch r.Method {
	case http.MethodGet:
		passphrase := strings.TrimSpace(r.URL.Query().Get("passphrase"))
		data, summary, err := s.encryptedVars.Get(name, passphrase)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"file": summary,
			"data": data,
		})
	case http.MethodDelete:
		if !s.encryptedVars.Delete(name) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "encrypted variable file not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
