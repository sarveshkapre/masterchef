package control

import (
	"encoding/json"
	"sort"
	"strings"
)

type DocsExampleVerificationReport struct {
	Checked  int      `json:"checked"`
	Passed   bool     `json:"passed"`
	Failures []string `json:"failures,omitempty"`
}

func VerifyActionDocExamples(items []ActionDoc, knownEndpoints []string) DocsExampleVerificationReport {
	report := DocsExampleVerificationReport{
		Checked:  len(items),
		Passed:   true,
		Failures: []string{},
	}
	endpointSet := map[string]struct{}{}
	for _, endpoint := range knownEndpoints {
		endpointSet[strings.TrimSpace(endpoint)] = struct{}{}
	}

	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			report.Failures = append(report.Failures, "action doc missing id")
			continue
		}
		if len(item.Endpoints) == 0 {
			report.Failures = append(report.Failures, item.ID+": endpoints are required")
		}
		for _, endpoint := range item.Endpoints {
			e := strings.TrimSpace(endpoint)
			if e == "" {
				report.Failures = append(report.Failures, item.ID+": endpoint cannot be empty")
				continue
			}
			if len(endpointSet) > 0 {
				if _, ok := endpointSet[e]; !ok {
					report.Failures = append(report.Failures, item.ID+": unknown endpoint "+e)
				}
			}
		}
		if raw := strings.TrimSpace(item.ExampleJSON); raw != "" && !json.Valid([]byte(raw)) {
			report.Failures = append(report.Failures, item.ID+": example_json is invalid json")
		}
	}
	sort.Strings(report.Failures)
	report.Passed = len(report.Failures) == 0
	if report.Passed {
		report.Failures = nil
	}
	return report
}
