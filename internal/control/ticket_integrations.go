package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type TicketIntegrationInput struct {
	Name       string `json:"name"`
	Provider   string `json:"provider"` // jira|servicenow|github|custom
	BaseURL    string `json:"base_url"`
	ProjectKey string `json:"project_key,omitempty"`
	Enabled    bool   `json:"enabled"`
}

type TicketIntegration struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Provider   string    `json:"provider"`
	BaseURL    string    `json:"base_url"`
	ProjectKey string    `json:"project_key,omitempty"`
	Enabled    bool      `json:"enabled"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ChangeTicketSyncInput struct {
	IntegrationID  string `json:"integration_id"`
	ChangeRecordID string `json:"change_record_id"`
	TicketID       string `json:"ticket_id"`
	Status         string `json:"status,omitempty"` // open|approved|rejected|implemented|failed
}

type ChangeTicketLink struct {
	ID             string    `json:"id"`
	IntegrationID  string    `json:"integration_id"`
	ChangeRecordID string    `json:"change_record_id"`
	TicketID       string    `json:"ticket_id"`
	TicketURL      string    `json:"ticket_url"`
	Status         string    `json:"status"`
	SyncedAt       time.Time `json:"synced_at"`
}

type TicketSyncResult struct {
	Linked bool             `json:"linked"`
	Reason string           `json:"reason,omitempty"`
	Link   ChangeTicketLink `json:"link,omitempty"`
}

type TicketIntegrationStore struct {
	mu           sync.RWMutex
	nextID       int64
	nextLinkID   int64
	integrations map[string]*TicketIntegration
	links        map[string]*ChangeTicketLink
}

func NewTicketIntegrationStore() *TicketIntegrationStore {
	return &TicketIntegrationStore{
		integrations: map[string]*TicketIntegration{},
		links:        map[string]*ChangeTicketLink{},
	}
}

func (s *TicketIntegrationStore) Upsert(in TicketIntegrationInput) (TicketIntegration, error) {
	name := strings.TrimSpace(in.Name)
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	baseURL := strings.TrimSpace(in.BaseURL)
	if name == "" || provider == "" || baseURL == "" {
		return TicketIntegration{}, errors.New("name, provider, and base_url are required")
	}
	switch provider {
	case "jira", "servicenow", "github", "custom":
	default:
		return TicketIntegration{}, errors.New("provider must be jira, servicenow, github, or custom")
	}
	item := TicketIntegration{
		Name:       name,
		Provider:   provider,
		BaseURL:    strings.TrimRight(baseURL, "/"),
		ProjectKey: strings.TrimSpace(in.ProjectKey),
		Enabled:    in.Enabled,
		UpdatedAt:  time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.integrations {
		if strings.EqualFold(existing.Name, item.Name) {
			item.ID = existing.ID
			s.integrations[item.ID] = &item
			return item, nil
		}
	}
	s.nextID++
	item.ID = "ticket-integration-" + itoa(s.nextID)
	s.integrations[item.ID] = &item
	return item, nil
}

func (s *TicketIntegrationStore) List() []TicketIntegration {
	s.mu.RLock()
	out := make([]TicketIntegration, 0, len(s.integrations))
	for _, item := range s.integrations {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *TicketIntegrationStore) Get(id string) (TicketIntegration, bool) {
	s.mu.RLock()
	item, ok := s.integrations[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return TicketIntegration{}, false
	}
	return *item, true
}

func (s *TicketIntegrationStore) SetEnabled(id string, enabled bool) (TicketIntegration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.integrations[strings.TrimSpace(id)]
	if !ok {
		return TicketIntegration{}, errors.New("ticket integration not found")
	}
	item.Enabled = enabled
	item.UpdatedAt = time.Now().UTC()
	return *item, nil
}

func (s *TicketIntegrationStore) ListLinks() []ChangeTicketLink {
	s.mu.RLock()
	out := make([]ChangeTicketLink, 0, len(s.links))
	for _, link := range s.links {
		out = append(out, *link)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].SyncedAt.After(out[j].SyncedAt) })
	return out
}

func (s *TicketIntegrationStore) Sync(in ChangeTicketSyncInput, changeRecordExists bool) TicketSyncResult {
	integrationID := strings.TrimSpace(in.IntegrationID)
	changeRecordID := strings.TrimSpace(in.ChangeRecordID)
	ticketID := strings.TrimSpace(in.TicketID)
	status := strings.ToLower(strings.TrimSpace(in.Status))
	if status == "" {
		status = "open"
	}
	if integrationID == "" || changeRecordID == "" || ticketID == "" {
		return TicketSyncResult{Linked: false, Reason: "integration_id, change_record_id, and ticket_id are required"}
	}
	if !changeRecordExists {
		return TicketSyncResult{Linked: false, Reason: "change record not found"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	integration, ok := s.integrations[integrationID]
	if !ok {
		return TicketSyncResult{Linked: false, Reason: "ticket integration not found"}
	}
	if !integration.Enabled {
		return TicketSyncResult{Linked: false, Reason: "ticket integration is disabled"}
	}
	key := integrationID + "|" + changeRecordID
	item, ok := s.links[key]
	if !ok {
		s.nextLinkID++
		item = &ChangeTicketLink{
			ID:             "ticket-link-" + itoa(s.nextLinkID),
			IntegrationID:  integrationID,
			ChangeRecordID: changeRecordID,
		}
	}
	item.TicketID = ticketID
	item.TicketURL = buildTicketURL(*integration, ticketID)
	item.Status = status
	item.SyncedAt = time.Now().UTC()
	s.links[key] = item
	return TicketSyncResult{Linked: true, Link: *item}
}

func buildTicketURL(integration TicketIntegration, ticketID string) string {
	if integration.ProjectKey != "" && !strings.Contains(ticketID, "-") {
		ticketID = integration.ProjectKey + "-" + ticketID
	}
	base := strings.TrimRight(integration.BaseURL, "/")
	switch integration.Provider {
	case "jira":
		return base + "/browse/" + ticketID
	case "servicenow":
		return base + "/nav_to.do?uri=task.do?sysparm_query=number=" + ticketID
	case "github":
		return base + "/issues/" + ticketID
	default:
		return base + "/" + ticketID
	}
}
