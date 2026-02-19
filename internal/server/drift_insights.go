package server

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/state"
)

type driftTrend struct {
	Key      string    `json:"key"`
	Count    int       `json:"count"`
	LastSeen time.Time `json:"last_seen"`
}

func (s *Server) handleDriftInsights(baseDir string) http.HandlerFunc {
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
		if hours > 24*30 {
			hours = 24 * 30
		}
		since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)

		runs, err := state.New(baseDir).ListRuns(5000)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		hostTrends := map[string]*driftTrend{}
		typeTrends := map[string]*driftTrend{}
		totalChanged := 0
		suppressedChanges := 0
		allowlistedChanges := 0
		failedRuns := 0
		for _, run := range runs {
			ref := run.StartedAt
			if ref.IsZero() {
				ref = run.EndedAt
			}
			if ref.IsZero() || ref.Before(since) {
				continue
			}
			if run.Status == state.RunFailed {
				failedRuns++
			}
			for _, res := range run.Results {
				if !res.Changed {
					continue
				}
				if s.driftPolicies != nil && s.driftPolicies.IsSuppressed(res.Host, res.Type, res.ResourceID, ref) {
					suppressedChanges++
					continue
				}
				if s.driftPolicies != nil && s.driftPolicies.IsAllowlisted(res.Host, res.Type, res.ResourceID, ref) {
					allowlistedChanges++
					continue
				}
				totalChanged++
				hostKey := strings.TrimSpace(res.Host)
				if hostKey == "" {
					hostKey = "unknown-host"
				}
				if hostTrends[hostKey] == nil {
					hostTrends[hostKey] = &driftTrend{Key: hostKey}
				}
				hostTrends[hostKey].Count++
				hostTrends[hostKey].LastSeen = maxTime(hostTrends[hostKey].LastSeen, ref)

				typeKey := strings.TrimSpace(strings.ToLower(res.Type))
				if typeKey == "" {
					typeKey = "unknown-type"
				}
				if typeTrends[typeKey] == nil {
					typeTrends[typeKey] = &driftTrend{Key: typeKey}
				}
				typeTrends[typeKey].Count++
				typeTrends[typeKey].LastSeen = maxTime(typeTrends[typeKey].LastSeen, ref)
			}
		}

		hostItems := sortDriftTrends(hostTrends, 10)
		typeItems := sortDriftTrends(typeTrends, 10)
		hints, remediations := driftHints(hostItems, typeItems, failedRuns)
		activeSuppressions := []any{}
		activeAllowlist := []any{}
		if s.driftPolicies != nil {
			for _, item := range s.driftPolicies.ListSuppressions(false) {
				activeSuppressions = append(activeSuppressions, item)
			}
			for _, item := range s.driftPolicies.ListAllowlist(false) {
				activeAllowlist = append(activeAllowlist, item)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"window_hours":            hours,
			"since":                   since,
			"total_changed_resources": totalChanged,
			"suppressed_changes":      suppressedChanges,
			"allowlisted_changes":     allowlistedChanges,
			"active_suppressions":     activeSuppressions,
			"active_allowlists":       activeAllowlist,
			"failed_runs":             failedRuns,
			"host_trends":             hostItems,
			"resource_type_trends":    typeItems,
			"root_cause_hints":        hints,
			"remediations":            remediations,
		})
	}
}

func sortDriftTrends(in map[string]*driftTrend, limit int) []driftTrend {
	out := make([]driftTrend, 0, len(in))
	for _, item := range in {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func driftHints(hosts, types []driftTrend, failedRuns int) ([]string, []string) {
	hints := make([]string, 0, 3)
	remediations := make([]string, 0, 4)
	if len(hosts) > 0 && hosts[0].Count >= 3 {
		hints = append(hints, "drift is concentrated on host "+hosts[0].Key)
		remediations = append(remediations, "inspect host-specific config overlays and local manual changes")
	}
	if len(types) > 0 && types[0].Key == "command" {
		hints = append(hints, "imperative command resources are the primary drift driver")
		remediations = append(remediations, "replace imperative commands with declarative file/package resources where possible")
	}
	if failedRuns > 0 {
		hints = append(hints, "failed runs correlate with drift spikes in the selected window")
		remediations = append(remediations, "compare failed/success runs with /v1/runs/compare and apply targeted retries")
	}
	if len(hints) == 0 {
		hints = append(hints, "no dominant drift root-cause signal detected")
		remediations = append(remediations, "continue monitoring drift trends and enforce periodic check/noop scans")
	}
	return hints, remediations
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
