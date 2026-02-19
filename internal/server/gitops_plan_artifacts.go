package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/policy"
)

func (s *Server) handleGitOpsPlanArtifactSign(baseDir string) http.HandlerFunc {
	type signReq struct {
		ConfigPath     string `json:"config_path"`
		PrivateKeyPath string `json:"private_key_path"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req signReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		configPath := strings.TrimSpace(req.ConfigPath)
		privPath := strings.TrimSpace(req.PrivateKeyPath)
		if configPath == "" || privPath == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path and private_key_path are required"})
			return
		}
		resolvedConfig := configPath
		if !filepath.IsAbs(resolvedConfig) {
			resolvedConfig = filepath.Join(baseDir, resolvedConfig)
		}
		resolvedPriv := privPath
		if !filepath.IsAbs(resolvedPriv) {
			resolvedPriv = filepath.Join(baseDir, resolvedPriv)
		}
		bundle, err := policy.Build(resolvedConfig)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		priv, err := policy.LoadPrivateKey(resolvedPriv)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := bundle.Sign(priv); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "gitops.plan_artifact.signed",
			Message: "signed gitops plan artifact generated",
			Fields: map[string]any{
				"config_path": bundle.ConfigPath,
				"config_sha":  bundle.ConfigSHA,
			},
		}, true)
		writeJSON(w, http.StatusOK, bundle)
	}
}

func (s *Server) handleGitOpsPlanArtifactVerify(baseDir string) http.HandlerFunc {
	type verifyReq struct {
		PublicKeyPath string        `json:"public_key_path"`
		Bundle        policy.Bundle `json:"bundle"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req verifyReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		pubPath := strings.TrimSpace(req.PublicKeyPath)
		if pubPath == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "public_key_path is required"})
			return
		}
		resolvedPub := pubPath
		if !filepath.IsAbs(resolvedPub) {
			resolvedPub = filepath.Join(baseDir, resolvedPub)
		}
		pub, err := policy.LoadPublicKey(resolvedPub)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := req.Bundle.Verify(pub); err != nil {
			writeJSON(w, http.StatusConflict, map[string]any{
				"verified": false,
				"error":    err.Error(),
			})
			return
		}
		s.recordEvent(control.Event{
			Type:    "gitops.plan_artifact.verified",
			Message: "signed gitops plan artifact verified",
			Fields: map[string]any{
				"config_path": req.Bundle.ConfigPath,
				"config_sha":  req.Bundle.ConfigSHA,
			},
		}, true)
		writeJSON(w, http.StatusOK, map[string]any{
			"verified": true,
			"bundle":   req.Bundle,
		})
	}
}
