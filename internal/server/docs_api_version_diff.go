package server

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleDocsAPIVersionDiff(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Baseline control.APISpec `json:"baseline"`
	}
	current := currentAPISpec()

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"current_spec":         current,
			"deprecation_timeline": docsDeprecationTimeline(current.Deprecations),
			"endpoint_count":       len(current.Endpoints),
		})
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		diff := control.DiffAPISpec(req.Baseline, current)
		writeJSON(w, http.StatusOK, map[string]any{
			"current_spec":         current,
			"diff":                 diff,
			"deprecation_timeline": docsDeprecationTimeline(current.Deprecations),
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func docsDeprecationTimeline(items []control.APIDeprecation) []control.APIDeprecation {
	out := append([]control.APIDeprecation{}, items...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].RemoveAfterVersion != out[j].RemoveAfterVersion {
			return out[i].RemoveAfterVersion < out[j].RemoveAfterVersion
		}
		return out[i].Endpoint < out[j].Endpoint
	})
	return out
}
