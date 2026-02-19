package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/testimpact"
)

func (s *Server) handleTestImpactAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ChangedFiles       []string `json:"changed_files"`
		AlwaysInclude      []string `json:"always_include,omitempty"`
		MaxTargetedPackage int      `json:"max_targeted_packages,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if len(req.ChangedFiles) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "changed_files is required"})
		return
	}
	report := testimpact.AnalyzeWithOptions(req.ChangedFiles, testimpact.AnalyzeOptions{
		AlwaysInclude:      req.AlwaysInclude,
		MaxTargetedPackage: req.MaxTargetedPackage,
	})
	writeJSON(w, http.StatusOK, report)
}
