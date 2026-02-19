package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handlePackageArtifacts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.packageRegistry.ListArtifacts())
	case http.MethodPost:
		var req control.PackageArtifactInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.packageRegistry.Publish(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "packages.artifact.published",
			Message: "module/provider package artifact published",
			Fields: map[string]any{
				"artifact_id": item.ID,
				"kind":        item.Kind,
				"name":        item.Name,
				"version":     item.Version,
				"signed":      item.Signed,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePackageArtifactAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/packages/artifacts/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "packages" || parts[2] != "artifacts" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.packageRegistry.GetArtifact(parts[3])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "package artifact not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handlePackageSigningPolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.packageRegistry.Policy())
	case http.MethodPost:
		var req control.PackageSigningPolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		policy := s.packageRegistry.SetPolicy(req)
		s.recordEvent(control.Event{
			Type:    "packages.signing_policy.updated",
			Message: "package signing policy updated",
			Fields: map[string]any{
				"require_signed":  policy.RequireSigned,
				"trusted_key_ids": policy.TrustedKeyIDs,
			},
		}, true)
		writeJSON(w, http.StatusOK, policy)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePackageVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.PackageVerificationInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result := s.packageRegistry.Verify(req)
	code := http.StatusOK
	if !result.Allowed {
		code = http.StatusConflict
	}
	writeJSON(w, code, result)
}

func (s *Server) handlePackageCertificationPolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.packageRegistry.CertificationPolicy())
	case http.MethodPost:
		var req control.PackageCertificationPolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		policy, err := s.packageRegistry.SetCertificationPolicy(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "packages.certification_policy.updated",
			Message: "package certification policy updated",
			Fields: map[string]any{
				"require_conformance": policy.RequireConformance,
				"min_test_pass_rate":  policy.MinTestPassRate,
				"max_high_vulns":      policy.MaxHighVulns,
				"max_critical_vulns":  policy.MaxCriticalVulns,
				"require_signed":      policy.RequireSigned,
			},
		}, true)
		writeJSON(w, http.StatusOK, policy)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePackageCertify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.PackageCertificationInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	report, err := s.packageRegistry.Certify(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "packages.certification.evaluated",
		Message: "package certification report generated",
		Fields: map[string]any{
			"report_id":   report.ID,
			"artifact_id": report.ArtifactID,
			"certified":   report.Certified,
			"tier":        report.Tier,
			"score":       report.Score,
		},
	}, true)
	status := http.StatusCreated
	if !report.Certified {
		status = http.StatusConflict
	}
	writeJSON(w, status, report)
}

func (s *Server) handlePackageCertifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.packageRegistry.ListCertifications())
}

func (s *Server) handlePackagePublicationCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.PackagePublicationCheckInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result := s.packageRegistry.PublicationGateCheck(req)
	code := http.StatusOK
	if !result.Allowed {
		code = http.StatusConflict
	}
	writeJSON(w, code, result)
}

func (s *Server) handlePackageMaintainerHealth(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.packageRegistry.ListMaintainerHealth())
	case http.MethodPost:
		var req control.MaintainerHealthInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		report, err := s.packageRegistry.UpsertMaintainerHealth(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "packages.maintainer.health.updated",
			Message: "maintainer health report updated",
			Fields: map[string]any{
				"maintainer":           report.Maintainer,
				"score":                report.Score,
				"tier":                 report.Tier,
				"test_pass_rate":       report.TestPassRate,
				"issue_latency_hours":  report.IssueLatencyHours,
				"release_cadence_days": report.ReleaseCadenceDays,
			},
		}, true)
		writeJSON(w, http.StatusCreated, report)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePackageMaintainerHealthAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/packages/maintainers/health/{maintainer}
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "packages" || parts[2] != "maintainers" || parts[3] != "health" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.packageRegistry.GetMaintainerHealth(parts[4])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "maintainer health report not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handlePackageProvenanceReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.packageRegistry.ProvenanceReport())
}
