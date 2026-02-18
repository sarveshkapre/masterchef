package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/state"
	"github.com/masterchef/masterchef/internal/storage"
)

type backupSnapshot struct {
	Version   string            `json:"version"`
	CreatedAt time.Time         `json:"created_at"`
	Runs      []state.RunRecord `json:"runs,omitempty"`
	Events    []control.Event   `json:"events,omitempty"`
}

var errInvalidBackupSnapshotPayload = errors.New("invalid backup snapshot payload")

func (s *Server) buildBackupSnapshot(baseDir string, includeRuns, includeEvents bool) (backupSnapshot, error) {
	snap := backupSnapshot{
		Version:   "v1",
		CreatedAt: time.Now().UTC(),
	}
	if includeRuns {
		runs, err := state.New(baseDir).ListRuns(100000)
		if err != nil {
			return backupSnapshot{}, err
		}
		snap.Runs = runs
	}
	if includeEvents {
		snap.Events = s.events.List()
	}
	return snap, nil
}

func (s *Server) putBackupSnapshot(prefix string, snap backupSnapshot) (storage.ObjectInfo, error) {
	payload, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	key := storage.TimestampedJSONKey(prefix, "snapshot")
	return s.objectStore.Put(key, payload, "application/json")
}

func (s *Server) getBackupSnapshot(key string) (backupSnapshot, storage.ObjectInfo, error) {
	if strings.TrimSpace(key) == "" {
		return backupSnapshot{}, storage.ObjectInfo{}, errors.New("key is required")
	}
	payload, obj, err := s.objectStore.Get(key)
	if err != nil {
		return backupSnapshot{}, storage.ObjectInfo{}, err
	}
	var snap backupSnapshot
	if err := json.Unmarshal(payload, &snap); err != nil {
		return backupSnapshot{}, storage.ObjectInfo{}, errInvalidBackupSnapshotPayload
	}
	return snap, obj, nil
}

func (s *Server) handleBackup(baseDir string) http.HandlerFunc {
	type reqBody struct {
		IncludeRuns   bool   `json:"include_runs"`
		IncludeEvents bool   `json:"include_events"`
		Prefix        string `json:"prefix"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.objectStore == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "object store unavailable"})
			return
		}
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		if !req.IncludeRuns && !req.IncludeEvents {
			req.IncludeRuns = true
			req.IncludeEvents = true
		}
		if strings.TrimSpace(req.Prefix) == "" {
			req.Prefix = "backups"
		}

		snap, err := s.buildBackupSnapshot(baseDir, req.IncludeRuns, req.IncludeEvents)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		obj, err := s.putBackupSnapshot(req.Prefix, snap)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"object":          obj,
			"snapshot_runs":   len(snap.Runs),
			"snapshot_events": len(snap.Events),
		})
	}
}

func (s *Server) handleDRDrill(baseDir string) http.HandlerFunc {
	type reqBody struct {
		IncludeRuns   bool   `json:"include_runs"`
		IncludeEvents bool   `json:"include_events"`
		Prefix        string `json:"prefix"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.objectStore == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "object store unavailable"})
			return
		}
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		if !req.IncludeRuns && !req.IncludeEvents {
			req.IncludeRuns = true
			req.IncludeEvents = true
		}
		if strings.TrimSpace(req.Prefix) == "" {
			req.Prefix = "backups/drill"
		}

		start := time.Now().UTC()
		snap, err := s.buildBackupSnapshot(baseDir, req.IncludeRuns, req.IncludeEvents)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		obj, err := s.putBackupSnapshot(req.Prefix, snap)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		verified, verifyObj, err := s.getBackupSnapshot(obj.Key)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if len(verified.Runs) != len(snap.Runs) || len(verified.Events) != len(snap.Events) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":              "drill verification mismatch",
				"expected_runs":      len(snap.Runs),
				"verified_runs":      len(verified.Runs),
				"expected_events":    len(snap.Events),
				"verified_events":    len(verified.Events),
				"snapshot_object":    obj,
				"verification_object": verifyObj,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":          "verified",
			"snapshot_object": obj,
			"verified_runs":   len(verified.Runs),
			"verified_events": len(verified.Events),
			"snapshot_version": verified.Version,
			"duration_ms":     time.Since(start).Milliseconds(),
		})
	}
}

func (s *Server) handleBackups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.objectStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "object store unavailable"})
		return
	}
	prefix := strings.TrimSpace(r.URL.Query().Get("prefix"))
	if prefix == "" {
		prefix = "backups"
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := s.objectStore.List(prefix, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleRestore(baseDir string) http.HandlerFunc {
	type reqBody struct {
		Key        string `json:"key"`
		VerifyOnly bool   `json:"verify_only"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.objectStore == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "object store unavailable"})
			return
		}
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		if strings.TrimSpace(req.Key) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
			return
		}
		snap, obj, err := s.getBackupSnapshot(req.Key)
		if err != nil {
			if errors.Is(err, errInvalidBackupSnapshotPayload) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		if req.VerifyOnly {
			writeJSON(w, http.StatusOK, map[string]any{
				"status":  "verified",
				"object":  obj,
				"runs":    len(snap.Runs),
				"events":  len(snap.Events),
				"version": snap.Version,
			})
			return
		}
		if err := state.New(baseDir).ReplaceRuns(snap.Runs); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.events.Replace(snap.Events)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":          "restored",
			"object":          obj,
			"restored_runs":   len(snap.Runs),
			"restored_events": len(snap.Events),
		})
	}
}
