package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type MigrationDeprecationInput struct {
	Name        string `json:"name"`
	Severity    string `json:"severity"` // low|medium|high|critical
	EOLDate     string `json:"eol_date,omitempty"`
	Replacement string `json:"replacement,omitempty"`
}

type MigrationSemanticCheck struct {
	Name       string `json:"name"`
	Expected   string `json:"expected"`
	Translated string `json:"translated"`
}

type MigrationAssessmentRequest struct {
	SourcePlatform string                      `json:"source_platform"` // chef|ansible|puppet|salt
	Workload       string                      `json:"workload,omitempty"`
	UsedFeatures   []string                    `json:"used_features,omitempty"`
	Deprecations   []MigrationDeprecationInput `json:"deprecations,omitempty"`
	SemanticChecks []MigrationSemanticCheck    `json:"semantic_checks,omitempty"`
}

type MigrationDiffEntry struct {
	Feature         string `json:"feature,omitempty"`
	Severity        string `json:"severity"` // info|warn|error
	Message         string `json:"message"`
	SuggestedAction string `json:"suggested_action,omitempty"`
}

type MigrationDeprecationRisk struct {
	Name           string `json:"name"`
	Severity       string `json:"severity"`
	EOLDate        string `json:"eol_date,omitempty"`
	DaysToEOL      int    `json:"days_to_eol,omitempty"`
	UrgencyScore   int    `json:"urgency_score"`
	Recommendation string `json:"recommendation,omitempty"`
}

type MigrationSemanticResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // pass|warn|fail
	Message string `json:"message"`
}

type MigrationAssessment struct {
	ID                string                     `json:"id"`
	CreatedAt         time.Time                  `json:"created_at"`
	SourcePlatform    string                     `json:"source_platform"`
	Workload          string                     `json:"workload,omitempty"`
	UsedFeatures      []string                   `json:"used_features,omitempty"`
	SupportedFeatures []string                   `json:"supported_features,omitempty"`
	Unsupported       []string                   `json:"unsupported_features,omitempty"`
	ParityScore       int                        `json:"parity_score"`
	RiskScore         int                        `json:"risk_score"`
	UrgencyScore      int                        `json:"urgency_score"`
	DiffReport        []MigrationDiffEntry       `json:"diff_report,omitempty"`
	DeprecationRisk   []MigrationDeprecationRisk `json:"deprecation_risk,omitempty"`
	SemanticResults   []MigrationSemanticResult  `json:"semantic_results,omitempty"`
	Recommendations   []string                   `json:"recommendations,omitempty"`
}

type MigrationStore struct {
	mu      sync.RWMutex
	nextID  int64
	reports map[string]MigrationAssessment
}

func NewMigrationStore() *MigrationStore {
	return &MigrationStore{
		reports: map[string]MigrationAssessment{},
	}
}

func (s *MigrationStore) Assess(req MigrationAssessmentRequest) (MigrationAssessment, error) {
	platform := normalizeFeature(req.SourcePlatform)
	if platform == "" {
		return MigrationAssessment{}, errors.New("source_platform is required")
	}
	capabilities, ok := migrationCapabilities[platform]
	if !ok {
		return MigrationAssessment{}, errors.New("unsupported source_platform")
	}

	used := normalizeList(req.UsedFeatures)
	supported := make([]string, 0, len(used))
	unsupported := make([]string, 0)
	diff := make([]MigrationDiffEntry, 0)
	for _, feature := range used {
		if _, exists := capabilities[feature]; exists {
			supported = append(supported, feature)
			continue
		}
		unsupported = append(unsupported, feature)
		diff = append(diff, MigrationDiffEntry{
			Feature:         feature,
			Severity:        "warn",
			Message:         "feature has no direct migration mapping in current policy model",
			SuggestedAction: "refactor with typed resources/templates or add provider/plugin shim",
		})
	}

	parityScore := 100
	if len(used) > 0 {
		parityScore = (len(supported)*100 + len(used)/2) / len(used)
	}

	semanticResults, semanticDiff, semanticFailures := evaluateSemanticChecks(req.SemanticChecks)
	diff = append(diff, semanticDiff...)

	deprecationRisk := evaluateDeprecationRisk(req.Deprecations)
	urgencyScore := 0
	deprecationRiskAccumulator := 0
	for _, item := range deprecationRisk {
		if item.UrgencyScore > urgencyScore {
			urgencyScore = item.UrgencyScore
		}
		deprecationRiskAccumulator += item.UrgencyScore
	}

	riskScore := len(unsupported)*12 + semanticFailures*20 + deprecationRiskAccumulator/4
	if riskScore > 100 {
		riskScore = 100
	}
	if riskScore < 0 {
		riskScore = 0
	}

	recommendations := buildMigrationRecommendations(unsupported, semanticFailures, urgencyScore)
	if len(recommendations) == 0 {
		recommendations = []string{"migration profile is healthy; proceed with staged rollout and semantic equivalence tests in CI"}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	report := MigrationAssessment{
		ID:                "migration-" + itoa(s.nextID),
		CreatedAt:         time.Now().UTC(),
		SourcePlatform:    platform,
		Workload:          strings.TrimSpace(req.Workload),
		UsedFeatures:      used,
		SupportedFeatures: supported,
		Unsupported:       unsupported,
		ParityScore:       parityScore,
		RiskScore:         riskScore,
		UrgencyScore:      urgencyScore,
		DiffReport:        diff,
		DeprecationRisk:   deprecationRisk,
		SemanticResults:   semanticResults,
		Recommendations:   recommendations,
	}
	s.reports[report.ID] = cloneMigrationReport(report)
	return cloneMigrationReport(report), nil
}

func (s *MigrationStore) List() []MigrationAssessment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MigrationAssessment, 0, len(s.reports))
	for _, item := range s.reports {
		out = append(out, cloneMigrationReport(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *MigrationStore) Get(id string) (MigrationAssessment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.reports[strings.TrimSpace(id)]
	if !ok {
		return MigrationAssessment{}, false
	}
	return cloneMigrationReport(item), true
}

func cloneMigrationReport(in MigrationAssessment) MigrationAssessment {
	out := in
	out.UsedFeatures = append([]string{}, in.UsedFeatures...)
	out.SupportedFeatures = append([]string{}, in.SupportedFeatures...)
	out.Unsupported = append([]string{}, in.Unsupported...)
	out.DiffReport = append([]MigrationDiffEntry{}, in.DiffReport...)
	out.DeprecationRisk = append([]MigrationDeprecationRisk{}, in.DeprecationRisk...)
	out.SemanticResults = append([]MigrationSemanticResult{}, in.SemanticResults...)
	out.Recommendations = append([]string{}, in.Recommendations...)
	return out
}

func evaluateSemanticChecks(checks []MigrationSemanticCheck) ([]MigrationSemanticResult, []MigrationDiffEntry, int) {
	results := make([]MigrationSemanticResult, 0, len(checks))
	diff := make([]MigrationDiffEntry, 0)
	failures := 0
	for _, check := range checks {
		name := strings.TrimSpace(check.Name)
		if name == "" {
			name = "unnamed-check"
		}
		expected := normalizeFeature(check.Expected)
		translated := normalizeFeature(check.Translated)
		switch {
		case expected == "" || translated == "":
			results = append(results, MigrationSemanticResult{
				Name:    name,
				Status:  "warn",
				Message: "semantic check is incomplete",
			})
			diff = append(diff, MigrationDiffEntry{
				Feature:         name,
				Severity:        "warn",
				Message:         "semantic equivalence check is incomplete",
				SuggestedAction: "provide both expected and translated behavior for deterministic comparison",
			})
		case expected == translated || strings.Contains(translated, expected):
			results = append(results, MigrationSemanticResult{
				Name:    name,
				Status:  "pass",
				Message: "translated behavior is semantically equivalent",
			})
		default:
			failures++
			results = append(results, MigrationSemanticResult{
				Name:    name,
				Status:  "fail",
				Message: "translated behavior diverges from expected semantics",
			})
			diff = append(diff, MigrationDiffEntry{
				Feature:         name,
				Severity:        "error",
				Message:         "behavior mismatch detected between source and translated policy",
				SuggestedAction: "update migration mapping or add compatibility shim before rollout",
			})
		}
	}
	return results, diff, failures
}

func evaluateDeprecationRisk(items []MigrationDeprecationInput) []MigrationDeprecationRisk {
	out := make([]MigrationDeprecationRisk, 0, len(items))
	now := time.Now().UTC()
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		severity := normalizeDeprecationSeverity(item.Severity)
		score := severityScore(severity)
		daysToEOL := 0
		eol := strings.TrimSpace(item.EOLDate)
		if eol != "" {
			if parsed, err := time.Parse("2006-01-02", eol); err == nil {
				daysToEOL = int(parsed.Sub(now).Hours() / 24)
				switch {
				case daysToEOL <= 0:
					score += 40
				case daysToEOL <= 30:
					score += 30
				case daysToEOL <= 90:
					score += 20
				case daysToEOL <= 180:
					score += 10
				}
			}
		}
		if score > 100 {
			score = 100
		}
		rec := "plan phased replacement and validate equivalent behavior"
		if strings.TrimSpace(item.Replacement) != "" {
			rec = "migrate to " + strings.TrimSpace(item.Replacement) + " with semantic regression checks"
		}
		out = append(out, MigrationDeprecationRisk{
			Name:           name,
			Severity:       severity,
			EOLDate:        eol,
			DaysToEOL:      daysToEOL,
			UrgencyScore:   score,
			Recommendation: rec,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UrgencyScore != out[j].UrgencyScore {
			return out[i].UrgencyScore > out[j].UrgencyScore
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func buildMigrationRecommendations(unsupported []string, semanticFailures, urgency int) []string {
	out := make([]string, 0, 4)
	if len(unsupported) > 0 {
		out = append(out, "close unsupported feature gaps with module/provider shims before production migration")
	}
	if semanticFailures > 0 {
		out = append(out, "block rollout until semantic equivalence failures are resolved")
	}
	switch {
	case urgency >= 80:
		out = append(out, "treat migration as urgent and execute phased cutover with rollback rehearsals")
	case urgency >= 50:
		out = append(out, "schedule migration within the next release cycle to reduce deprecation risk")
	}
	return out
}

func normalizeFeature(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.ReplaceAll(raw, "_", " ")
	raw = strings.Join(strings.Fields(raw), " ")
	return raw
}

func normalizeList(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		n := normalizeFeature(item)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func normalizeDeprecationSeverity(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}

func severityScore(severity string) int {
	switch severity {
	case "critical":
		return 55
	case "high":
		return 40
	case "medium":
		return 25
	default:
		return 10
	}
}

var migrationCapabilities = map[string]map[string]struct{}{
	"chef": {
		"recipes":             {},
		"resources":           {},
		"roles":               {},
		"environments":        {},
		"data bags":           {},
		"policyfiles":         {},
		"run lists":           {},
		"handlers":            {},
		"chef search":         {},
		"encrypted data bags": {},
	},
	"ansible": {
		"playbooks":         {},
		"roles":             {},
		"collections":       {},
		"static inventory":  {},
		"dynamic inventory": {},
		"vault":             {},
		"check mode":        {},
		"diff mode":         {},
		"handlers":          {},
		"delegate to":       {},
		"become":            {},
	},
	"puppet": {
		"resource dsl":        {},
		"catalog compilation": {},
		"hiera":               {},
		"facter":              {},
		"enc":                 {},
		"modules":             {},
		"filebucket":          {},
		"noop":                {},
		"orchestrator":        {},
		"agent certs":         {},
	},
	"salt": {
		"states":     {},
		"highstate":  {},
		"pillar":     {},
		"grains":     {},
		"salt mine":  {},
		"beacons":    {},
		"reactor":    {},
		"salt ssh":   {},
		"masterless": {},
		"scheduler":  {},
	},
}
