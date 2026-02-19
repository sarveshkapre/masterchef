package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleDocsGenerate(w http.ResponseWriter, r *http.Request) {
	build := func(req control.DocumentationGenerateInput) control.DocumentationArtifact {
		return control.GenerateDocumentation(req, s.packageRegistry.ListArtifacts(), currentAPISpec().Endpoints)
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, build(control.DocumentationGenerateInput{
			Format:           "markdown",
			IncludePackages:  true,
			IncludePolicyAPI: true,
		}))
	case http.MethodPost:
		var req control.DocumentationGenerateInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item := build(req)
		s.recordEvent(control.Event{
			Type:    "docs.generated",
			Message: "generated docs artifact",
			Fields: map[string]any{
				"format":   item.Format,
				"sections": item.Sections,
				"counts":   item.Counts,
			},
		}, true)
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDocsExampleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	report := control.VerifyActionDocExamples(s.actionDocs.List(), currentAPISpec().Endpoints)
	code := http.StatusOK
	if !report.Passed {
		code = http.StatusConflict
	}
	writeJSON(w, code, report)
}
