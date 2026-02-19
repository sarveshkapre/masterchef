package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleContentChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.contentChannels.ListChannels())
}

func (s *Server) handleContentChannelPolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		channel := strings.TrimSpace(r.URL.Query().Get("channel"))
		item, err := s.contentChannels.GetPolicy(channel)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	case http.MethodPost:
		var req control.ChannelSyncPolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.contentChannels.SetPolicy(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleContentChannelRemotes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		org := strings.TrimSpace(r.URL.Query().Get("organization"))
		channel := strings.TrimSpace(r.URL.Query().Get("channel"))
		writeJSON(w, http.StatusOK, s.contentChannels.ListRemotes(org, channel))
	case http.MethodPost:
		var req control.OrgSyncRemoteInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.contentChannels.UpsertRemote(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleContentChannelRemoteAction(w http.ResponseWriter, r *http.Request) {
	// /v1/packages/content-channels/remotes/{id}[/rotate-token]
	parts := splitPath(r.URL.Path)
	if len(parts) < 5 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid content channel remote path"})
		return
	}
	id := strings.TrimSpace(parts[4])
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "remote id is required"})
		return
	}
	if len(parts) == 5 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, err := s.contentChannels.GetRemote(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) == 6 && parts[5] == "rotate-token" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			APIToken string `json:"api_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.contentChannels.RotateRemoteToken(id, req.APIToken)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown remote action"})
}
