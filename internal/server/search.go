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

type searchResult struct {
	Type        string         `json:"type"`
	ID          string         `json:"id,omitempty"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Score       int            `json:"score"`
	Source      string         `json:"source,omitempty"`
	Fields      map[string]any `json:"fields,omitempty"`
}

func (s *Server) handleSearch(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if query == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q is required"})
			return
		}
		limit := 25
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > 200 {
			limit = 200
		}

		allowedTypes := parseSearchTypes(r)
		results := s.search(baseDir, query, allowedTypes, limit)
		writeJSON(w, http.StatusOK, map[string]any{
			"query":         query,
			"count":         len(results),
			"limit":         limit,
			"allowed_types": sortedTypeList(allowedTypes),
			"items":         results,
		})
	}
}

func parseSearchTypes(r *http.Request) map[string]struct{} {
	out := map[string]struct{}{}
	add := func(raw string) {
		for _, part := range strings.Split(raw, ",") {
			t := strings.TrimSpace(strings.ToLower(part))
			switch t {
			case "host", "service", "run", "policy", "module":
				out[t] = struct{}{}
			}
		}
	}
	add(r.URL.Query().Get("types"))
	for _, raw := range r.URL.Query()["type"] {
		add(raw)
	}
	if len(out) == 0 {
		for _, t := range []string{"host", "service", "run", "policy", "module"} {
			out[t] = struct{}{}
		}
	}
	return out
}

func sortedTypeList(in map[string]struct{}) []string {
	out := make([]string, 0, len(in))
	for t := range in {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func (s *Server) search(baseDir, query string, allowedTypes map[string]struct{}, limit int) []searchResult {
	query = strings.TrimSpace(strings.ToLower(query))
	now := time.Now().UTC()

	results := make([]searchResult, 0, limit*2)
	seen := map[string]struct{}{}
	appendResult := func(item searchResult) {
		key := item.Type + "|" + item.ID + "|" + strings.ToLower(item.Title)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		results = append(results, item)
	}

	containsType := func(t string) bool {
		_, ok := allowedTypes[t]
		return ok
	}

	if containsType("run") || containsType("host") {
		runs, _ := state.New(baseDir).ListRuns(2000)
		for _, run := range runs {
			if containsType("run") {
				if score, ok := queryScore(query, run.ID, string(run.Status)); ok {
					appendResult(searchResult{
						Type:        "run",
						ID:          run.ID,
						Title:       run.ID,
						Description: "status=" + string(run.Status),
						Score:       score + freshnessScore(now.Sub(run.StartedAt), 20),
						Source:      "runs",
						Fields: map[string]any{
							"status":     run.Status,
							"started_at": run.StartedAt,
							"ended_at":   run.EndedAt,
						},
					})
				}
			}
			if containsType("host") {
				for _, res := range run.Results {
					host := strings.TrimSpace(res.Host)
					if host == "" {
						continue
					}
					if score, ok := queryScore(query, host, res.ResourceID, res.Type); ok {
						appendResult(searchResult{
							Type:        "host",
							ID:          host,
							Title:       host,
							Description: "seen in run " + run.ID,
							Score:       score + freshnessScore(now.Sub(run.StartedAt), 15),
							Source:      "runs",
							Fields: map[string]any{
								"run_id":      run.ID,
								"resource_id": res.ResourceID,
								"resource":    res.Type,
							},
						})
					}
				}
			}
		}
	}

	if containsType("host") || containsType("service") {
		events := s.events.Query(control.EventQuery{Limit: 5000, Desc: true})
		for _, event := range events {
			if containsType("host") {
				if host := firstNonEmptyField(event.Fields, "host", "node", "hostname"); host != "" {
					if score, ok := queryScore(query, host, event.Type, event.Message); ok {
						appendResult(searchResult{
							Type:        "host",
							ID:          normalizeWorkload(host),
							Title:       host,
							Description: event.Type,
							Score:       score + freshnessScore(now.Sub(event.Time), 25),
							Source:      "events",
							Fields: map[string]any{
								"event_type": event.Type,
								"time":       event.Time,
							},
						})
					}
				}
			}
			if containsType("service") {
				name := firstNonEmptyField(event.Fields, "service", "application", "app", "workload")
				if name == "" {
					continue
				}
				if score, ok := queryScore(query, name, event.Type, event.Message); ok {
					appendResult(searchResult{
						Type:        "service",
						ID:          normalizeWorkload(name),
						Title:       name,
						Description: event.Type,
						Score:       score + freshnessScore(now.Sub(event.Time), 25),
						Source:      "events",
						Fields: map[string]any{
							"event_type": event.Type,
							"time":       event.Time,
						},
					})
				}
			}
		}
	}

	if containsType("policy") {
		for _, tpl := range s.templates.List() {
			if score, ok := queryScore(query, tpl.ID, tpl.Name, tpl.Description, tpl.ConfigPath); ok {
				appendResult(searchResult{
					Type:        "policy",
					ID:          tpl.ID,
					Title:       tpl.Name,
					Description: "template policy",
					Score:       score,
					Source:      "templates",
					Fields: map[string]any{
						"config_path": tpl.ConfigPath,
					},
				})
			}
		}
		for _, rb := range s.runbooks.List() {
			if score, ok := queryScore(query, rb.ID, rb.Name, rb.Description, rb.ConfigPath, rb.Owner, rb.RiskLevel, strings.Join(rb.Tags, " ")); ok {
				appendResult(searchResult{
					Type:        "policy",
					ID:          rb.ID,
					Title:       rb.Name,
					Description: "runbook policy",
					Score:       score,
					Source:      "runbooks",
					Fields: map[string]any{
						"status":      rb.Status,
						"risk_level":  rb.RiskLevel,
						"target_type": rb.TargetType,
					},
				})
			}
		}
		for _, assoc := range s.assocs.List() {
			title := assoc.TargetKind + ":" + assoc.TargetName
			if score, ok := queryScore(query, assoc.ID, title, assoc.ConfigPath, assoc.TargetKind, assoc.TargetName); ok {
				appendResult(searchResult{
					Type:        "policy",
					ID:          assoc.ID,
					Title:       title,
					Description: "scheduled policy association",
					Score:       score,
					Source:      "associations",
					Fields: map[string]any{
						"target_kind": assoc.TargetKind,
						"target_name": assoc.TargetName,
					},
				})
			}
		}
	}

	if containsType("module") {
		for _, pack := range s.solutionPacks.List() {
			if score, ok := queryScore(query, pack.ID, pack.Name, pack.Description, pack.Category, strings.Join(pack.RecommendedTags, " ")); ok {
				appendResult(searchResult{
					Type:        "module",
					ID:          pack.ID,
					Title:       pack.Name,
					Description: "solution pack",
					Score:       score,
					Source:      "solution_packs",
					Fields: map[string]any{
						"category": pack.Category,
					},
				})
			}
		}
		for _, tpl := range s.workspaceTemplates.List() {
			if score, ok := queryScore(query, tpl.ID, tpl.Name, tpl.Description, tpl.Pattern, strings.Join(tpl.RecommendedTags, " ")); ok {
				appendResult(searchResult{
					Type:        "module",
					ID:          tpl.ID,
					Title:       tpl.Name,
					Description: "workspace template",
					Score:       score,
					Source:      "workspace_templates",
					Fields: map[string]any{
						"pattern": tpl.Pattern,
					},
				})
			}
		}
		for _, tpl := range s.useCaseTemplates.List() {
			if score, ok := queryScore(query, tpl.ID, tpl.Name, tpl.Description, tpl.Scenario, strings.Join(tpl.RecommendedTags, " ")); ok {
				appendResult(searchResult{
					Type:        "module",
					ID:          tpl.ID,
					Title:       tpl.Name,
					Description: "use-case template",
					Score:       score,
					Source:      "use_case_templates",
					Fields: map[string]any{
						"scenario": tpl.Scenario,
					},
				})
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Type != results[j].Type {
			return results[i].Type < results[j].Type
		}
		return strings.ToLower(results[i].Title) < strings.ToLower(results[j].Title)
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

func queryScore(query string, fields ...string) (int, bool) {
	best := 0
	for _, field := range fields {
		text := strings.TrimSpace(strings.ToLower(field))
		if text == "" {
			continue
		}
		switch {
		case text == query:
			if best < 120 {
				best = 120
			}
		case strings.HasPrefix(text, query):
			if best < 95 {
				best = 95
			}
		case strings.Contains(text, query):
			if best < 70 {
				best = 70
			}
		}
	}
	return best, best > 0
}

func freshnessScore(age time.Duration, max int) int {
	if max <= 0 {
		return 0
	}
	if age <= 0 {
		return max
	}
	hours := age.Hours()
	switch {
	case hours <= 1:
		return max
	case hours <= 6:
		return max - max/6
	case hours <= 24:
		return max / 2
	case hours <= 72:
		return max / 4
	default:
		return 0
	}
}
