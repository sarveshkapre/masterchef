package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

type migrationAssessRequest struct {
	SourcePlatform string   `json:"source_platform"`
	Workload       string   `json:"workload,omitempty"`
	UsedFeatures   []string `json:"used_features,omitempty"`
	Deprecations   []struct {
		Name        string `json:"name"`
		Severity    string `json:"severity"`
		EOLDate     string `json:"eol_date,omitempty"`
		Replacement string `json:"replacement,omitempty"`
	} `json:"deprecations,omitempty"`
	SemanticChecks []struct {
		Name       string `json:"name"`
		Expected   string `json:"expected"`
		Translated string `json:"translated"`
	} `json:"semantic_checks,omitempty"`
}

func (r migrationAssessRequest) toControl() control.MigrationAssessmentRequest {
	deprecations := make([]control.MigrationDeprecationInput, 0, len(r.Deprecations))
	for _, item := range r.Deprecations {
		deprecations = append(deprecations, control.MigrationDeprecationInput{
			Name:        item.Name,
			Severity:    item.Severity,
			EOLDate:     item.EOLDate,
			Replacement: item.Replacement,
		})
	}
	checks := make([]control.MigrationSemanticCheck, 0, len(r.SemanticChecks))
	for _, item := range r.SemanticChecks {
		checks = append(checks, control.MigrationSemanticCheck{
			Name:       item.Name,
			Expected:   item.Expected,
			Translated: item.Translated,
		})
	}
	return control.MigrationAssessmentRequest{
		SourcePlatform: r.SourcePlatform,
		Workload:       r.Workload,
		UsedFeatures:   r.UsedFeatures,
		Deprecations:   deprecations,
		SemanticChecks: checks,
	}
}

func migrationAssessmentEvent(report control.MigrationAssessment) control.Event {
	return control.Event{
		Type:    "migration.assess.completed",
		Message: "migration assessment generated",
		Fields: map[string]any{
			"report_id":       report.ID,
			"source_platform": report.SourcePlatform,
			"parity_score":    report.ParityScore,
			"risk_score":      report.RiskScore,
			"urgency_score":   report.UrgencyScore,
		},
	}
}

func (s *Server) handleMigrationAssess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req migrationAssessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	report, err := s.migrations.Assess(req.toControl())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(migrationAssessmentEvent(report), true)
	writeJSON(w, http.StatusCreated, report)
}

func (s *Server) handleMigrationReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.migrations.List())
}

func (s *Server) handleMigrationReportByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid migration report path"})
		return
	}
	id := parts[3]
	item, ok := s.migrations.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "migration report not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
