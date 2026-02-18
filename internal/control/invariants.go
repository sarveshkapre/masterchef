package control

import (
	"strings"
)

type Invariant struct {
	Name       string  `json:"name"`
	Field      string  `json:"field"`
	Comparator string  `json:"comparator"` // eq|ne|gt|gte|lt|lte
	Value      float64 `json:"value"`
	Severity   string  `json:"severity"` // info|warning|critical
}

type InvariantResult struct {
	Name       string  `json:"name"`
	Passed     bool    `json:"passed"`
	Severity   string  `json:"severity"`
	Field      string  `json:"field"`
	Observed   float64 `json:"observed"`
	Expected   float64 `json:"expected"`
	Comparator string  `json:"comparator"`
	Message    string  `json:"message"`
}

type InvariantReport struct {
	Pass         bool              `json:"pass"`
	Results      []InvariantResult `json:"results"`
	FailedCount  int               `json:"failed_count"`
	CriticalFail int               `json:"critical_fail_count"`
}

func EvaluateInvariants(invariants []Invariant, observed map[string]float64) InvariantReport {
	results := make([]InvariantResult, 0, len(invariants))
	failed := 0
	criticalFailed := 0
	for _, inv := range invariants {
		sev := normalizeInvariantSeverity(inv.Severity)
		obs := observed[strings.TrimSpace(inv.Field)]
		passed := compareInvariant(obs, inv.Comparator, inv.Value)
		msg := "invariant passed"
		if !passed {
			msg = "invariant failed"
			failed++
			if sev == "critical" {
				criticalFailed++
			}
		}
		results = append(results, InvariantResult{
			Name:       strings.TrimSpace(inv.Name),
			Passed:     passed,
			Severity:   sev,
			Field:      strings.TrimSpace(inv.Field),
			Observed:   obs,
			Expected:   inv.Value,
			Comparator: strings.TrimSpace(inv.Comparator),
			Message:    msg,
		})
	}
	return InvariantReport{
		Pass:         criticalFailed == 0,
		Results:      results,
		FailedCount:  failed,
		CriticalFail: criticalFailed,
	}
}

func compareInvariant(obs float64, comparator string, expected float64) bool {
	switch strings.ToLower(strings.TrimSpace(comparator)) {
	case "eq":
		return obs == expected
	case "ne":
		return obs != expected
	case "gt":
		return obs > expected
	case "gte":
		return obs >= expected
	case "lt":
		return obs < expected
	case "lte":
		return obs <= expected
	default:
		return false
	}
}

func normalizeInvariantSeverity(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "critical":
		return "critical"
	case "warning":
		return "warning"
	default:
		return "info"
	}
}
