package control

import (
	"testing"
	"time"
)

func TestBulkManagerPreviewLifecycle(t *testing.T) {
	m := NewBulkManager(2 * time.Second)
	preview := m.SavePreview("test", []BulkOperationPreview{
		{
			Operation: BulkOperation{Action: "schedule.disable", TargetType: "schedule", TargetID: "sch-1"},
			Ready:     true,
		},
	}, nil)
	if preview.Token == "" {
		t.Fatalf("expected preview token")
	}
	got, err := m.GetPreview(preview.Token)
	if err != nil {
		t.Fatalf("get preview failed: %v", err)
	}
	if got.Token != preview.Token {
		t.Fatalf("unexpected preview token: %s", got.Token)
	}
	consumed, err := m.ConsumePreview(preview.Token)
	if err != nil {
		t.Fatalf("consume preview failed: %v", err)
	}
	if consumed.Token != preview.Token {
		t.Fatalf("unexpected consumed token: %s", consumed.Token)
	}
	if _, err := m.GetPreview(preview.Token); err == nil {
		t.Fatalf("expected consumed preview to be unavailable")
	}
}
