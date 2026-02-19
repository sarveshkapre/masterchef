package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleEncryptedSecrets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.encryptedSecrets.List())
	case http.MethodPost:
		var req control.EncryptedSecretUpsertInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.encryptedSecrets.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEncryptedSecretExpired(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.encryptedSecrets.Expired())
}

func (s *Server) handleEncryptedSecretAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/secrets/encrypted-store/items/{name}[/resolve|rotate]
	if len(parts) < 5 || parts[0] != "v1" || parts[1] != "secrets" || parts[2] != "encrypted-store" || parts[3] != "items" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	name := parts[4]
	if len(parts) == 5 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, ok := s.encryptedSecrets.Get(name)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "secret not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) == 6 && parts[5] == "resolve" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		result, err := s.encryptedSecrets.Resolve(name)
		if err != nil {
			code := http.StatusNotFound
			if err.Error() == "secret expired" {
				code = http.StatusGone
			}
			writeJSON(w, code, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if len(parts) == 6 && parts[5] == "rotate" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req control.EncryptedSecretRotateInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.encryptedSecrets.Rotate(name, req)
		if err != nil {
			code := http.StatusBadRequest
			if err.Error() == "secret not found" {
				code = http.StatusNotFound
			}
			if err.Error() == "secret expired" {
				code = http.StatusGone
			}
			writeJSON(w, code, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}
