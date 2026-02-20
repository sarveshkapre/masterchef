package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleProviderConformanceFixtures(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 100
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		provider := strings.TrimSpace(r.URL.Query().Get("provider"))
		writeJSON(w, http.StatusOK, map[string]any{"items": s.providerFixtureHarness.ListFixtures(provider, limit)})
	case http.MethodPost:
		var req control.ProviderTestFixture
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.providerFixtureHarness.UpsertFixture(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProviderConformanceFixtureAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/providers/conformance/fixtures/{id}
	if len(parts) != 5 || !strings.EqualFold(parts[0], "v1") || !strings.EqualFold(parts[1], "providers") || !strings.EqualFold(parts[2], "conformance") || !strings.EqualFold(parts[3], "fixtures") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid provider conformance fixture path"})
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, err := s.providerFixtureHarness.GetFixture(parts[4])
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleProviderConformanceHarnessRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 100
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		provider := strings.TrimSpace(r.URL.Query().Get("provider"))
		writeJSON(w, http.StatusOK, map[string]any{"items": s.providerFixtureHarness.ListRuns(provider, limit)})
	case http.MethodPost:
		var req control.ProviderHarnessRunInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		run, err := s.providerFixtureHarness.Run(req, s.providerConformance)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		code := http.StatusOK
		if run.Status == "fail" {
			code = http.StatusConflict
		}
		writeJSON(w, code, run)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProviderConformanceHarnessRunAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/providers/conformance/harness/runs/{id}
	if len(parts) != 6 || !strings.EqualFold(parts[0], "v1") || !strings.EqualFold(parts[1], "providers") || !strings.EqualFold(parts[2], "conformance") || !strings.EqualFold(parts[3], "harness") || !strings.EqualFold(parts[4], "runs") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid provider conformance harness run path"})
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, err := s.providerFixtureHarness.GetRun(parts[5])
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
