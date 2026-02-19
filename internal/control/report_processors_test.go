package control

import "testing"

func TestReportProcessorUpsertAndDispatch(t *testing.T) {
	store := NewReportProcessorStore()
	item, err := store.Upsert(ReportProcessorPluginInput{
		Name:           "incident-webhook",
		Kind:           "webhook",
		Destination:    "https://hooks.example/report",
		TimeoutSeconds: 5,
		RetryLimit:     2,
		RedactFields:   []string{"secret", "token"},
		Enabled:        true,
	})
	if err != nil {
		t.Fatalf("upsert report processor failed: %v", err)
	}
	result := store.Dispatch(ReportProcessorDispatchInput{
		RunID:        "run-123",
		Status:       "failed",
		Severity:     "high",
		ProcessorIDs: []string{item.ID},
		Payload: map[string]any{
			"message": "run failed",
			"secret":  "abc",
		},
	})
	if !result.Dispatched {
		t.Fatalf("expected dispatched result, got %+v", result)
	}
	if len(result.Outcomes) != 1 || !result.Outcomes[0].Accepted {
		t.Fatalf("expected accepted outcome, got %+v", result.Outcomes)
	}
	if result.Outcomes[0].Preview["secret"] != "***redacted***" {
		t.Fatalf("expected secret redaction, got %+v", result.Outcomes[0].Preview)
	}
}

func TestReportProcessorDispatchDisabled(t *testing.T) {
	store := NewReportProcessorStore()
	item, err := store.Upsert(ReportProcessorPluginInput{
		Name:        "archive-processor",
		Kind:        "objectstore",
		Destination: "s3://bucket/reports",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("upsert report processor failed: %v", err)
	}
	if _, err := store.SetEnabled(item.ID, false); err != nil {
		t.Fatalf("disable report processor failed: %v", err)
	}
	result := store.Dispatch(ReportProcessorDispatchInput{
		RunID: "run-456",
	})
	if result.Dispatched {
		t.Fatalf("expected dispatch blocked with only disabled processors, got %+v", result)
	}
}
