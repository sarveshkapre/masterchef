package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/policy"
)

func (s *Server) handleOfflineMode(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.offline.Mode())
	case http.MethodPost:
		var req control.OfflineModeConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item := s.offline.SetMode(req)
		s.recordEvent(control.Event{
			Type:    "control.offline.mode",
			Message: "offline mode updated",
			Fields: map[string]any{
				"enabled":     item.Enabled,
				"air_gapped":  item.AirGapped,
				"mirror_path": item.MirrorPath,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOfflineBundles(baseDir string) http.HandlerFunc {
	type createReq struct {
		Items          []string `json:"items"`
		Artifacts      []string `json:"artifacts,omitempty"`
		PrivateKeyPath string   `json:"private_key_path,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			limit := 100
			if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n > 0 {
					limit = n
				}
			}
			writeJSON(w, http.StatusOK, s.offline.ListBundles(limit))
		case http.MethodPost:
			var req createReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			if len(req.Items) == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "items are required"})
				return
			}
			manifestBytes, _ := json.Marshal(map[string]any{"items": req.Items, "artifacts": req.Artifacts})
			sum := sha256.Sum256(manifestBytes)
			manifestSHA := "sha256:" + base64.StdEncoding.EncodeToString(sum[:])

			signed := false
			signature := ""
			if keyPath := strings.TrimSpace(req.PrivateKeyPath); keyPath != "" {
				resolved := keyPath
				if !filepath.IsAbs(resolved) {
					resolved = filepath.Join(baseDir, resolved)
				}
				priv, err := policy.LoadPrivateKey(resolved)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				bundle := policy.Bundle{ConfigPath: "offline-bundle", ConfigSHA: manifestSHA}
				if err := bundle.Sign(priv); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				signed = true
				signature = bundle.Signature
			}
			item, err := s.offline.CreateBundle(control.OfflineBundleInput{
				ManifestSHA: manifestSHA,
				Items:       req.Items,
				Artifacts:   req.Artifacts,
				Signed:      signed,
				Signature:   signature,
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
}

func (s *Server) handleOfflineMirrors(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.offline.ListMirrors())
	case http.MethodPost:
		var req control.OfflineMirrorInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.offline.UpsertMirror(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "control.offline.mirror.upserted",
			Message: "offline mirror configuration updated",
			Fields: map[string]any{
				"mirror_id":   item.ID,
				"name":        item.Name,
				"upstream":    item.Upstream,
				"mirror_path": item.MirrorPath,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOfflineMirrorAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/offline/mirrors/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "offline" || parts[2] != "mirrors" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.offline.GetMirror(parts[3])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "offline mirror not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleOfflineMirrorSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.OfflineMirrorSyncInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	item, err := s.offline.SyncMirror(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "control.offline.mirror.sync",
		Message: "offline mirror sync evaluated",
		Fields: map[string]any{
			"mirror_id":        item.MirrorID,
			"artifact_count":   item.ArtifactCount,
			"synced_artifacts": item.SyncedArtifacts,
			"status":           item.Status,
		},
	}, true)
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleOfflineBundleVerify(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			BundleID      string `json:"bundle_id"`
			PublicKeyPath string `json:"public_key_path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		bundle, ok := s.offline.GetBundle(req.BundleID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "offline bundle not found"})
			return
		}
		if !bundle.Signed {
			writeJSON(w, http.StatusConflict, control.OfflineBundleVerifyResult{BundleID: bundle.ID, Verified: false, Reason: "bundle is unsigned"})
			return
		}
		resolved := strings.TrimSpace(req.PublicKeyPath)
		if resolved == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "public_key_path is required"})
			return
		}
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(baseDir, resolved)
		}
		pub, err := policy.LoadPublicKey(resolved)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		sigBundle := policy.Bundle{ConfigPath: "offline-bundle", ConfigSHA: bundle.ManifestSHA, Signature: bundle.Signature}
		if err := sigBundle.Verify(pub); err != nil {
			writeJSON(w, http.StatusConflict, control.OfflineBundleVerifyResult{BundleID: bundle.ID, Verified: false, Reason: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, control.OfflineBundleVerifyResult{BundleID: bundle.ID, Verified: true, Reason: "signature verified"})
	}
}
