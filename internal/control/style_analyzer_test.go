package control

import "testing"

func TestStyleAnalyzerPolicyErrors(t *testing.T) {
	analyzer := NewStyleAnalyzer()
	report, err := analyzer.Analyze(StyleAnalysisInput{
		Kind: "policy",
		Content: `
rules:
  - deny: true # TODO tighten
`,
	})
	if err != nil {
		t.Fatalf("analyze policy failed: %v", err)
	}
	if report.Pass {
		t.Fatalf("expected policy report to fail due to missing name: %+v", report)
	}
	if len(report.Issues) == 0 {
		t.Fatalf("expected policy issues")
	}
}

func TestStyleAnalyzerProviderTimeoutRule(t *testing.T) {
	analyzer := NewStyleAnalyzer()
	report, err := analyzer.Analyze(StyleAnalysisInput{
		Kind: "provider",
		Content: `
hooks:
  command: /usr/bin/custom
`,
	})
	if err != nil {
		t.Fatalf("analyze provider failed: %v", err)
	}
	found := false
	for _, item := range report.Issues {
		if item.RuleID == "provider-command-timeout" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected provider timeout issue, got %+v", report)
	}
}
