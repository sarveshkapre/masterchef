package control

import "testing"

func TestTicketIntegrationSync(t *testing.T) {
	store := NewTicketIntegrationStore()
	integration, err := store.Upsert(TicketIntegrationInput{
		Name:       "jira-prod",
		Provider:   "jira",
		BaseURL:    "https://tickets.example.com",
		ProjectKey: "OPS",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("upsert ticket integration failed: %v", err)
	}
	result := store.Sync(ChangeTicketSyncInput{
		IntegrationID:  integration.ID,
		ChangeRecordID: "cr-123",
		TicketID:       "42",
		Status:         "approved",
	}, true)
	if !result.Linked {
		t.Fatalf("expected linked ticket sync result, got %+v", result)
	}
	if result.Link.TicketURL == "" {
		t.Fatalf("expected ticket url in sync result, got %+v", result.Link)
	}
}

func TestTicketIntegrationSyncRejectsDisabled(t *testing.T) {
	store := NewTicketIntegrationStore()
	integration, err := store.Upsert(TicketIntegrationInput{
		Name:     "custom-disabled",
		Provider: "custom",
		BaseURL:  "https://custom.example.com/tickets",
		Enabled:  false,
	})
	if err != nil {
		t.Fatalf("upsert ticket integration failed: %v", err)
	}
	result := store.Sync(ChangeTicketSyncInput{
		IntegrationID:  integration.ID,
		ChangeRecordID: "cr-456",
		TicketID:       "A-1",
	}, true)
	if result.Linked {
		t.Fatalf("expected sync rejection for disabled integration, got %+v", result)
	}
}
