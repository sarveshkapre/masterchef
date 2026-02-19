package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleComplianceProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.compliance.ListProfiles())
	case http.MethodPost:
		var req control.ComplianceProfileInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.compliance.CreateProfile(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "compliance.profile.created",
			Message: "compliance profile created",
			Fields: map[string]any{
				"profile_id": item.ID,
				"framework":  item.Framework,
				"controls":   len(item.Controls),
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleComplianceProfileAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/compliance/profiles/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "compliance" || parts[2] != "profiles" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.compliance.GetProfile(parts[3])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "compliance profile not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleComplianceScans(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.compliance.ListScans())
	case http.MethodPost:
		var req control.ComplianceScanInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		scan, err := s.compliance.RunScan(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "compliance.scan.completed",
			Message: "compliance scan completed",
			Fields: map[string]any{
				"scan_id":    scan.ID,
				"profile_id": scan.ProfileID,
				"target":     scan.TargetKind + "/" + scan.TargetName,
				"status":     scan.Status,
				"score":      scan.Score,
			},
		}, true)
		writeJSON(w, http.StatusCreated, scan)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleComplianceScanAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/compliance/scans/{id} or /v1/compliance/scans/{id}/evidence
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "compliance" || parts[2] != "scans" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if len(parts) == 4 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		scan, ok := s.compliance.GetScan(parts[3])
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "compliance scan not found"})
			return
		}
		writeJSON(w, http.StatusOK, scan)
		return
	}
	if len(parts) == 5 && parts[4] == "evidence" && r.Method == http.MethodGet {
		format := strings.TrimSpace(r.URL.Query().Get("format"))
		content, contentType, err := s.compliance.ExportEvidence(parts[3], format)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
}

func (s *Server) handleComplianceContinuous(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.compliance.ListContinuousConfigs())
	case http.MethodPost:
		var req control.ComplianceContinuousInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		cfg, err := s.compliance.UpsertContinuousConfig(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "compliance.continuous.configured",
			Message: "continuous compliance config upserted",
			Fields: map[string]any{
				"config_id":  cfg.ID,
				"profile_id": cfg.ProfileID,
				"target":     cfg.TargetKind + "/" + cfg.TargetName,
				"enabled":    cfg.Enabled,
			},
		}, true)
		writeJSON(w, http.StatusCreated, cfg)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleComplianceContinuousAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/compliance/continuous/{id}/run
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "compliance" || parts[2] != "continuous" || parts[4] != "run" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	scan, cfg, err := s.compliance.RunContinuousScan(parts[3])
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "compliance.continuous.scan",
		Message: "continuous compliance scan executed",
		Fields: map[string]any{
			"config_id": cfg.ID,
			"scan_id":   scan.ID,
			"status":    scan.Status,
			"score":     scan.Score,
		},
	}, true)
	writeJSON(w, http.StatusOK, map[string]any{
		"config": cfg,
		"scan":   scan,
	})
}
