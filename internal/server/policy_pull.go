package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/policy"
)

func (s *Server) handlePolicyPullSources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.policyPull.ListSources())
	case http.MethodPost:
		var req control.PolicyPullSourceInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.policyPull.CreateSource(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "policy.pull.source.created",
			Message: "policy pull source created",
			Fields: map[string]any{
				"source_id":         item.ID,
				"type":              item.Type,
				"require_signature": item.RequireSignature,
				"enabled":           item.Enabled,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePolicyPullSourceAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/policy/pull/sources/{id}
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "policy" || parts[2] != "pull" || parts[3] != "sources" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.policyPull.GetSource(parts[4])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy pull source not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handlePolicyPullExecute(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req control.PolicyPullExecuteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		source, ok := s.policyPull.GetSource(req.SourceID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy pull source not found"})
			return
		}
		if !source.Enabled {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "policy pull source is disabled"})
			return
		}

		switch source.Type {
		case control.PolicyPullSourceTypeControlPlane:
			configPath := strings.TrimSpace(req.ConfigPath)
			if configPath == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path is required for control_plane source"})
				return
			}
			resolved := configPath
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(baseDir, resolved)
			}
			bundle, err := policy.Build(resolved)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			result := s.policyPull.RecordResult(control.PolicyPullResultInput{
				SourceID:         source.ID,
				SourceType:       source.Type,
				Revision:         req.Revision,
				ConfigPath:       bundle.ConfigPath,
				ConfigSHA:        bundle.ConfigSHA,
				Status:           "pulled",
				Verified:         true,
				Message:          "policy pulled from control plane",
				RequireSignature: false,
			})
			s.recordEvent(control.Event{
				Type:    "policy.pull.completed",
				Message: "policy pulled from control plane source",
				Fields: map[string]any{
					"source_id":   source.ID,
					"result_id":   result.ID,
					"config_path": result.ConfigPath,
				},
			}, true)
			writeJSON(w, http.StatusOK, result)
		case control.PolicyPullSourceTypeGitSigned:
			bundle := policy.Bundle{
				ConfigPath: strings.TrimSpace(req.Bundle.ConfigPath),
				ConfigSHA:  strings.TrimSpace(req.Bundle.ConfigSHA),
				Signature:  strings.TrimSpace(req.Bundle.Signature),
			}
			if bundle.ConfigPath == "" || bundle.ConfigSHA == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bundle.config_path and bundle.config_sha are required for git_signed source"})
				return
			}
			verified := false
			status := "pulled"
			message := "policy pulled from signed git source"
			if source.RequireSignature {
				pubPath := source.PublicKeyPath
				if !filepath.IsAbs(pubPath) {
					pubPath = filepath.Join(baseDir, pubPath)
				}
				pub, err := policy.LoadPublicKey(pubPath)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				if err := bundle.Verify(pub); err != nil {
					status = "rejected"
					message = "policy signature verification failed"
					result := s.policyPull.RecordResult(control.PolicyPullResultInput{
						SourceID:         source.ID,
						SourceType:       source.Type,
						Revision:         req.Revision,
						ConfigPath:       bundle.ConfigPath,
						ConfigSHA:        bundle.ConfigSHA,
						Status:           status,
						Verified:         false,
						Message:          message,
						RequireSignature: true,
					})
					writeJSON(w, http.StatusConflict, map[string]any{
						"error":  err.Error(),
						"result": result,
					})
					return
				}
				verified = true
			}
			result := s.policyPull.RecordResult(control.PolicyPullResultInput{
				SourceID:         source.ID,
				SourceType:       source.Type,
				Revision:         req.Revision,
				ConfigPath:       bundle.ConfigPath,
				ConfigSHA:        bundle.ConfigSHA,
				Status:           status,
				Verified:         verified,
				Message:          message,
				RequireSignature: source.RequireSignature,
			})
			s.recordEvent(control.Event{
				Type:    "policy.pull.completed",
				Message: "policy pulled from git source",
				Fields: map[string]any{
					"source_id": source.ID,
					"result_id": result.ID,
					"revision":  result.Revision,
					"verified":  result.Verified,
				},
			}, true)
			writeJSON(w, http.StatusOK, result)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported policy pull source type"})
		}
	}
}

func (s *Server) handlePolicyPullResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	writeJSON(w, http.StatusOK, s.policyPull.ListResults(limit))
}
