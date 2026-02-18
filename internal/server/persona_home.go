package server

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/state"
)

type homeCard struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Summary  string         `json:"summary"`
	Severity string         `json:"severity,omitempty"` // info|warning|critical
	Fields   map[string]any `json:"fields,omitempty"`
}

func (s *Server) handlePersonaHome(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		persona := normalizePersona(r.URL.Query().Get("persona"))
		if persona == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "persona must be one of: sre, platform, release, service-owner"})
			return
		}
		owner := strings.TrimSpace(r.URL.Query().Get("owner"))
		hours := 24
		if raw := strings.TrimSpace(r.URL.Query().Get("hours")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				hours = n
			}
		}
		cards, actions := s.personaCards(persona, owner, hours, baseDir)
		writeJSON(w, http.StatusOK, map[string]any{
			"persona":      persona,
			"owner":        owner,
			"window_hours": hours,
			"generated_at": time.Now().UTC(),
			"cards":        cards,
			"actions":      actions,
		})
	}
}

func normalizePersona(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "sre":
		return "sre"
	case "platform":
		return "platform"
	case "release":
		return "release"
	case "service-owner", "service_owner", "owner":
		return "service-owner"
	default:
		return ""
	}
}

func (s *Server) personaCards(persona, owner string, hours int, baseDir string) ([]homeCard, []string) {
	queue := s.queue.ControlStatus()
	alerts := s.alerts.Summary()
	workloads := s.computeWorkloadViews(5, time.Now().UTC().Add(-24*time.Hour))
	changeRecords := s.changeRecords.List()
	release := s.releaseSummary(baseDir, hours)

	switch persona {
	case "sre":
		return []homeCard{
				{
					ID:       "alert-inbox",
					Title:    "Alert Inbox",
					Summary:  "Open alerts and suppressions requiring operator triage.",
					Severity: ternarySeverity(alerts.Open > 0, "warning", "info"),
					Fields: map[string]any{
						"open":                alerts.Open,
						"acknowledged":        alerts.Acknowledged,
						"active_suppressions": alerts.ActiveSuppressions,
					},
				},
				{
					ID:       "queue-pressure",
					Title:    "Queue Pressure",
					Summary:  "Dispatch backlog and run concurrency status.",
					Severity: ternarySeverity(queue.Pending > s.backlogThreshold || queue.Paused, "warning", "info"),
					Fields: map[string]any{
						"pending": queue.Pending,
						"running": queue.Running,
						"paused":  queue.Paused,
					},
				},
				{
					ID:      "top-workloads",
					Title:   "Top Risk Workloads",
					Summary: "Workloads ranked by alert/failure signals.",
					Fields: map[string]any{
						"items": workloads,
					},
				},
			}, []string{
				"triage open alerts and clear stale suppressions",
				"review high-risk workloads before further rollouts",
				"resume queue dispatch if paused outside maintenance",
			}
	case "platform":
		migrations := s.migrations.List()
		schema := s.schemaMigs.Status()
		channels := s.channels.List()
		topMigration := map[string]any{}
		if len(migrations) > 0 {
			topMigration = map[string]any{
				"id":            migrations[0].ID,
				"source":        migrations[0].SourcePlatform,
				"parity_score":  migrations[0].ParityScore,
				"risk_score":    migrations[0].RiskScore,
				"urgency_score": migrations[0].UrgencyScore,
			}
		}
		return []homeCard{
				{
					ID:      "migration-risk",
					Title:   "Migration Risk",
					Summary: "Latest migration assessment and urgency signals.",
					Fields: map[string]any{
						"latest": topMigration,
						"count":  len(migrations),
					},
				},
				{
					ID:      "schema-evolution",
					Title:   "Schema Evolution",
					Summary: "Current control-plane schema version and migration history.",
					Fields: map[string]any{
						"current_version": schema.CurrentVersion,
						"history_count":   len(schema.History),
					},
				},
				{
					ID:      "release-channels",
					Title:   "Release Channels",
					Summary: "Current control/agent channel assignments.",
					Fields: map[string]any{
						"assignments": channels,
					},
				},
			}, []string{
				"resolve migration assessments with high urgency scores",
				"verify schema upgrade plans are stepwise and reversible",
				"audit channel assignments before promoting candidate builds",
			}
	default: // release, service-owner
		recentProposed := 0
		for _, rec := range changeRecords {
			if rec.Status == control.ChangeRecordProposed {
				recentProposed++
			}
		}
		runbooks := s.runbooks.List()
		if persona == "service-owner" {
			runbooks = filterRunbooksByOwner(runbooks, owner)
		}
		sort.Slice(runbooks, func(i, j int) bool {
			return runbooks[i].UpdatedAt.After(runbooks[j].UpdatedAt)
		})
		limitRunbooks := runbooks
		if len(limitRunbooks) > 10 {
			limitRunbooks = limitRunbooks[:10]
		}

		cards := []homeCard{
			{
				ID:       "release-risk",
				Title:    "Release Risk",
				Summary:  "Run success/failure and latent risk over the selected release window.",
				Severity: release.RiskLevel,
				Fields: map[string]any{
					"total_runs":        release.TotalRuns,
					"succeeded_runs":    release.SucceededRuns,
					"failed_runs":       release.FailedRuns,
					"latent_risk_score": release.RiskScore,
					"window_hours":      hours,
				},
			},
			{
				ID:       "change-approvals",
				Title:    "Change Approvals",
				Summary:  "Change records waiting for decision or execution completion.",
				Severity: ternarySeverity(recentProposed > 0, "warning", "info"),
				Fields: map[string]any{
					"pending_approvals": recentProposed,
					"total_records":     len(changeRecords),
				},
			},
			{
				ID:      "runbook-catalog",
				Title:   "Runbook Catalog",
				Summary: "Runbooks relevant to this operational persona.",
				Fields: map[string]any{
					"owner_filter": owner,
					"items":        limitRunbooks,
				},
			},
		}
		actions := []string{
			"review failed runs before next rollout wave",
			"close pending change approvals blocking release progress",
			"revalidate runbook freshness for critical workflows",
		}
		if persona == "service-owner" {
			actions = append(actions, "set owner query parameter to scope runbook ownership views")
		}
		return cards, actions
	}
}

func filterRunbooksByOwner(items []control.Runbook, owner string) []control.Runbook {
	owner = strings.TrimSpace(strings.ToLower(owner))
	if owner == "" {
		return items
	}
	out := make([]control.Runbook, 0, len(items))
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item.Owner)) == owner {
			out = append(out, item)
		}
	}
	return out
}

func ternarySeverity(cond bool, whenTrue, whenFalse string) string {
	if cond {
		return whenTrue
	}
	return whenFalse
}

type releaseSummary struct {
	TotalRuns       int
	SucceededRuns   int
	FailedRuns      int
	RiskScore       int
	RiskLevel       string
	ChangedResource int
}

func (s *Server) releaseSummary(baseDir string, hours int) releaseSummary {
	if hours <= 0 {
		hours = 24
	}
	windowStart := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	runs, _ := state.New(baseDir).ListRuns(1000)
	out := releaseSummary{}
	for _, run := range runs {
		ref := run.StartedAt
		if ref.IsZero() {
			ref = run.EndedAt
		}
		if ref.IsZero() || ref.Before(windowStart) {
			continue
		}
		out.TotalRuns++
		if run.Status == state.RunSucceeded {
			out.SucceededRuns++
		}
		if run.Status == state.RunFailed {
			out.FailedRuns++
		}
		for _, res := range run.Results {
			if res.Changed {
				out.ChangedResource++
			}
		}
	}
	failRate := 0.0
	if out.TotalRuns > 0 {
		failRate = float64(out.FailedRuns) / float64(out.TotalRuns)
	}
	out.RiskScore = int(failRate * 100)
	if out.RiskScore >= 60 {
		out.RiskLevel = "critical"
	} else if out.RiskScore >= 30 {
		out.RiskLevel = "warning"
	} else {
		out.RiskLevel = "info"
	}
	return out
}
