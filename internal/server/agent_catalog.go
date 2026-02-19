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

func (s *Server) handleAgentCatalogs(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			limit := 100
			if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n > 0 {
					limit = n
				}
			}
			writeJSON(w, http.StatusOK, s.agentCatalogs.ListCatalogs(limit))
		case http.MethodPost:
			var req control.AgentCatalogCompileInput
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			configPath := strings.TrimSpace(req.ConfigPath)
			if configPath == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path is required"})
				return
			}
			resolvedConfig := configPath
			if !filepath.IsAbs(resolvedConfig) {
				resolvedConfig = filepath.Join(baseDir, resolvedConfig)
			}
			bundle, err := policy.Build(resolvedConfig)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}

			signed := false
			signature := ""
			if keyPath := strings.TrimSpace(req.PrivateKeyPath); keyPath != "" {
				resolvedKey := keyPath
				if !filepath.IsAbs(resolvedKey) {
					resolvedKey = filepath.Join(baseDir, resolvedKey)
				}
				priv, err := policy.LoadPrivateKey(resolvedKey)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				if err := bundle.Sign(priv); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				signed = true
				signature = bundle.Signature
			}

			item, err := s.agentCatalogs.CreateCatalog(control.AgentCompiledCatalog{
				ConfigPath:  bundle.ConfigPath,
				PolicyGroup: req.PolicyGroup,
				AgentIDs:    req.AgentIDs,
				ConfigSHA:   bundle.ConfigSHA,
				Signature:   signature,
				Signed:      signed,
			})
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			s.recordEvent(control.Event{
				Type:    "agents.catalog.compiled",
				Message: "agent catalog compiled and cached",
				Fields: map[string]any{
					"catalog_id":   item.ID,
					"config_path":  item.ConfigPath,
					"policy_group": item.PolicyGroup,
					"signed":       item.Signed,
				},
			}, true)
			writeJSON(w, http.StatusCreated, item)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleAgentCatalogAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/agents/catalogs/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "agents" || parts[2] != "catalogs" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.agentCatalogs.GetCatalog(parts[3])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "catalog not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleAgentCatalogReplay(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req control.AgentCatalogReplayInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		catalog, ok := s.agentCatalogs.GetCatalog(req.CatalogID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "catalog not found"})
			return
		}
		agentID := strings.TrimSpace(req.AgentID)
		if agentID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id is required"})
			return
		}
		if len(catalog.AgentIDs) > 0 && !containsString(catalog.AgentIDs, agentID) {
			result := s.agentCatalogs.RecordReplay(control.AgentCatalogReplayRecord{
				CatalogID:    catalog.ID,
				AgentID:      agentID,
				Allowed:      false,
				Verified:     false,
				Reason:       "catalog is not targeted to this agent",
				Disconnected: req.Disconnected,
			})
			writeJSON(w, http.StatusConflict, result)
			return
		}

		allowed := false
		verified := false
		reason := ""
		if catalog.Signed {
			pubPath := strings.TrimSpace(req.PublicKeyPath)
			if pubPath == "" {
				reason = "public_key_path is required for signed catalog replay"
			} else {
				resolvedPub := pubPath
				if !filepath.IsAbs(resolvedPub) {
					resolvedPub = filepath.Join(baseDir, resolvedPub)
				}
				pub, err := policy.LoadPublicKey(resolvedPub)
				if err != nil {
					reason = err.Error()
				} else {
					bundle := policy.Bundle{
						ConfigPath: catalog.ConfigPath,
						ConfigSHA:  catalog.ConfigSHA,
						Signature:  catalog.Signature,
					}
					if err := bundle.Verify(pub); err != nil {
						reason = err.Error()
					} else {
						allowed = true
						verified = true
						reason = "signed catalog verified"
					}
				}
			}
		} else {
			if req.AllowUnsigned {
				allowed = true
				reason = "unsigned catalog replay explicitly allowed"
			} else {
				reason = "catalog is unsigned and allow_unsigned is false"
			}
		}

		record := s.agentCatalogs.RecordReplay(control.AgentCatalogReplayRecord{
			CatalogID:    catalog.ID,
			AgentID:      agentID,
			Allowed:      allowed,
			Verified:     verified,
			Reason:       reason,
			Disconnected: req.Disconnected,
		})
		code := http.StatusOK
		if !allowed {
			code = http.StatusConflict
		}
		writeJSON(w, code, record)
	}
}

func (s *Server) handleAgentCatalogReplays(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, s.agentCatalogs.ListReplays(limit))
}

func containsString(items []string, needle string) bool {
	needle = strings.TrimSpace(needle)
	for _, item := range items {
		if strings.TrimSpace(item) == needle {
			return true
		}
	}
	return false
}
