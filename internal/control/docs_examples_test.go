package control

import "testing"

func TestVerifyActionDocExamples(t *testing.T) {
	items := []ActionDoc{
		{
			ID:          "ok",
			Endpoints:   []string{"GET /v1/docs/actions"},
			ExampleJSON: `{"ok":true}`,
		},
		{
			ID:          "bad-json",
			Endpoints:   []string{"POST /v1/unknown"},
			ExampleJSON: `{"broken":`,
		},
	}
	report := VerifyActionDocExamples(items, []string{"GET /v1/docs/actions"})
	if report.Passed {
		t.Fatalf("expected verification to fail")
	}
	if len(report.Failures) < 2 {
		t.Fatalf("expected multiple failures, got %+v", report.Failures)
	}
}

func TestVerifyActionDocExamplesPass(t *testing.T) {
	items := []ActionDoc{
		{
			ID:          "ok",
			Endpoints:   []string{"GET /v1/docs/actions"},
			ExampleJSON: `{"ok":true}`,
		},
	}
	report := VerifyActionDocExamples(items, []string{"GET /v1/docs/actions"})
	if !report.Passed {
		t.Fatalf("expected verification to pass: %+v", report)
	}
}
