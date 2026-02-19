package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/state"
)

type driftHistoryEntry struct {
	RunID        string    `json:"run_id"`
	RunStatus    string    `json:"run_status"`
	RunStarted   time.Time `json:"run_started_at"`
	RunEnded     time.Time `json:"run_ended_at"`
	ResourceID   string    `json:"resource_id"`
	ResourceType string    `json:"resource_type"`
	Host         string    `json:"host"`
	Changed      bool      `json:"changed"`
	Skipped      bool      `json:"skipped"`
	Message      string    `json:"message,omitempty"`
	Suppressed   bool      `json:"suppressed,omitempty"`
	Allowlisted  bool      `json:"allowlisted,omitempty"`
}

func (s *Server) handleDriftHistory(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		hours := 24
		if raw := strings.TrimSpace(r.URL.Query().Get("hours")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				hours = n
			}
		}
		if hours > 24*90 {
			hours = 24 * 90
		}
		limit := 500
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > 10000 {
			limit = 10000
		}
		hostFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("host")))
		typeFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
		resourceFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("resource_id")))
		includeUnchanged := parseBoolQuery(r.URL.Query().Get("include_unchanged"))
		includeSuppressed := parseBoolQuery(r.URL.Query().Get("include_suppressed"))
		includeAllowlisted := parseBoolQuery(r.URL.Query().Get("include_allowlisted"))

		since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
		runs, err := state.New(baseDir).ListRuns(5000)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		items := make([]driftHistoryEntry, 0, limit)
		for _, run := range runs {
			ref := run.StartedAt
			if ref.IsZero() {
				ref = run.EndedAt
			}
			if ref.IsZero() || ref.Before(since) {
				continue
			}
			for _, res := range run.Results {
				if !includeUnchanged && !res.Changed {
					continue
				}
				host := strings.ToLower(strings.TrimSpace(res.Host))
				resType := strings.ToLower(strings.TrimSpace(res.Type))
				resID := strings.ToLower(strings.TrimSpace(res.ResourceID))
				if hostFilter != "" && hostFilter != host {
					continue
				}
				if typeFilter != "" && typeFilter != resType {
					continue
				}
				if resourceFilter != "" && resourceFilter != resID {
					continue
				}
				entry := driftHistoryEntry{
					RunID:        run.ID,
					RunStatus:    string(run.Status),
					RunStarted:   run.StartedAt,
					RunEnded:     run.EndedAt,
					ResourceID:   res.ResourceID,
					ResourceType: res.Type,
					Host:         res.Host,
					Changed:      res.Changed,
					Skipped:      res.Skipped,
					Message:      res.Message,
				}
				if s.driftPolicies != nil {
					entry.Suppressed = s.driftPolicies.IsSuppressed(res.Host, res.Type, res.ResourceID, ref)
					entry.Allowlisted = s.driftPolicies.IsAllowlisted(res.Host, res.Type, res.ResourceID, ref)
				}
				if !includeSuppressed && entry.Suppressed {
					continue
				}
				if !includeAllowlisted && entry.Allowlisted {
					continue
				}
				items = append(items, entry)
				if len(items) >= limit {
					break
				}
			}
			if len(items) >= limit {
				break
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"window_hours":        hours,
			"since":               since,
			"count":               len(items),
			"include_unchanged":   includeUnchanged,
			"include_suppressed":  includeSuppressed,
			"include_allowlisted": includeAllowlisted,
			"items":               items,
		})
	}
}
