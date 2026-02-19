package control

import (
	"errors"
	"sort"
	"strings"
)

type StyleRule struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"` // policy|module|provider|all
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type StyleIssue struct {
	RuleID     string `json:"rule_id"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Line       int    `json:"line"`
	Suggestion string `json:"suggestion,omitempty"`
}

type StyleAnalysisInput struct {
	Kind    string `json:"kind"` // policy|module|provider
	Content string `json:"content"`
	Source  string `json:"source,omitempty"`
}

type StyleAnalysisReport struct {
	Kind      string       `json:"kind"`
	Source    string       `json:"source,omitempty"`
	Pass      bool         `json:"pass"`
	Score     int          `json:"score"`
	Issues    []StyleIssue `json:"issues,omitempty"`
	RuleCount int          `json:"rule_count"`
}

type StyleAnalyzer struct {
	rules []StyleRule
}

func NewStyleAnalyzer() *StyleAnalyzer {
	return &StyleAnalyzer{
		rules: []StyleRule{
			{
				ID:          "style-tabs",
				Kind:        "all",
				Severity:    "warning",
				Title:       "Tab characters",
				Description: "Tabs reduce deterministic rendering; prefer spaces.",
			},
			{
				ID:          "style-todo",
				Kind:        "all",
				Severity:    "warning",
				Title:       "Unresolved TODO/FIXME",
				Description: "Shipping TODO/FIXME often indicates incomplete production logic.",
			},
			{
				ID:          "policy-missing-name",
				Kind:        "policy",
				Severity:    "error",
				Title:       "Missing policy name",
				Description: "Policy definitions should declare a deterministic name.",
			},
			{
				ID:          "module-missing-description",
				Kind:        "module",
				Severity:    "warning",
				Title:       "Missing module description",
				Description: "Modules should include concise description metadata.",
			},
			{
				ID:          "provider-command-timeout",
				Kind:        "provider",
				Severity:    "error",
				Title:       "Command without timeout",
				Description: "Provider command hooks should declare timeout safeguards.",
			},
		},
	}
}

func (a *StyleAnalyzer) Rules() []StyleRule {
	out := make([]StyleRule, len(a.rules))
	copy(out, a.rules)
	return out
}

func (a *StyleAnalyzer) Analyze(in StyleAnalysisInput) (StyleAnalysisReport, error) {
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	if kind != "policy" && kind != "module" && kind != "provider" {
		return StyleAnalysisReport{}, errors.New("kind must be policy, module, or provider")
	}
	content := in.Content
	if strings.TrimSpace(content) == "" {
		return StyleAnalysisReport{}, errors.New("content is required")
	}

	issues := make([]StyleIssue, 0)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lineNo := i + 1
		lower := strings.ToLower(line)
		if strings.Contains(line, "\t") {
			issues = append(issues, StyleIssue{
				RuleID:     "style-tabs",
				Severity:   "warning",
				Message:    "line contains tab character",
				Line:       lineNo,
				Suggestion: "replace tabs with spaces",
			})
		}
		if strings.Contains(lower, "todo") || strings.Contains(lower, "fixme") {
			issues = append(issues, StyleIssue{
				RuleID:     "style-todo",
				Severity:   "warning",
				Message:    "line contains unresolved TODO/FIXME",
				Line:       lineNo,
				Suggestion: "resolve TODO/FIXME before release",
			})
		}
		if kind == "provider" && strings.Contains(lower, "command:") && !strings.Contains(lower, "timeout") {
			issues = append(issues, StyleIssue{
				RuleID:     "provider-command-timeout",
				Severity:   "error",
				Message:    "provider command declared without timeout",
				Line:       lineNo,
				Suggestion: "add timeout guard for provider command execution",
			})
		}
	}

	lowerContent := strings.ToLower(content)
	switch kind {
	case "policy":
		if !strings.Contains(lowerContent, "name:") {
			issues = append(issues, StyleIssue{
				RuleID:     "policy-missing-name",
				Severity:   "error",
				Message:    "policy content missing name field",
				Line:       1,
				Suggestion: "add top-level name: <policy-id>",
			})
		}
	case "module":
		if !strings.Contains(lowerContent, "description:") {
			issues = append(issues, StyleIssue{
				RuleID:     "module-missing-description",
				Severity:   "warning",
				Message:    "module metadata missing description",
				Line:       1,
				Suggestion: "add description for module intent and boundaries",
			})
		}
	}

	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Line != issues[j].Line {
			return issues[i].Line < issues[j].Line
		}
		return issues[i].RuleID < issues[j].RuleID
	})

	score := 100
	for _, item := range issues {
		if item.Severity == "error" {
			score -= 20
		} else {
			score -= 8
		}
	}
	if score < 0 {
		score = 0
	}
	pass := true
	for _, item := range issues {
		if item.Severity == "error" {
			pass = false
			break
		}
	}

	return StyleAnalysisReport{
		Kind:      kind,
		Source:    strings.TrimSpace(in.Source),
		Pass:      pass,
		Score:     score,
		Issues:    issues,
		RuleCount: len(a.rules),
	}, nil
}
