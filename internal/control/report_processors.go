package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ReportProcessorPluginInput struct {
	Name           string   `json:"name"`
	Kind           string   `json:"kind"` // webhook|queue|objectstore|plugin
	Destination    string   `json:"destination"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
	RetryLimit     int      `json:"retry_limit,omitempty"`
	RedactFields   []string `json:"redact_fields,omitempty"`
	Enabled        bool     `json:"enabled"`
}

type ReportProcessorPlugin struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Kind           string    `json:"kind"`
	Destination    string    `json:"destination"`
	TimeoutSeconds int       `json:"timeout_seconds"`
	RetryLimit     int       `json:"retry_limit"`
	RedactFields   []string  `json:"redact_fields,omitempty"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ReportProcessorDispatchInput struct {
	RunID        string         `json:"run_id"`
	Status       string         `json:"status,omitempty"`
	Severity     string         `json:"severity,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
	ProcessorIDs []string       `json:"processor_ids,omitempty"`
}

type ReportProcessorDispatchOutcome struct {
	ProcessorID   string         `json:"processor_id"`
	ProcessorName string         `json:"processor_name"`
	Kind          string         `json:"kind"`
	Accepted      bool           `json:"accepted"`
	Reason        string         `json:"reason,omitempty"`
	DeliveredAt   time.Time      `json:"delivered_at,omitempty"`
	Preview       map[string]any `json:"preview,omitempty"`
}

type ReportProcessorDispatchResult struct {
	RunID         string                           `json:"run_id"`
	Status        string                           `json:"status,omitempty"`
	Severity      string                           `json:"severity,omitempty"`
	Dispatched    bool                             `json:"dispatched"`
	Outcomes      []ReportProcessorDispatchOutcome `json:"outcomes,omitempty"`
	CheckedAt     time.Time                        `json:"checked_at"`
	BlockedReason string                           `json:"blocked_reason,omitempty"`
}

type ReportProcessorStore struct {
	mu      sync.RWMutex
	nextID  int64
	plugins map[string]*ReportProcessorPlugin
}

func NewReportProcessorStore() *ReportProcessorStore {
	return &ReportProcessorStore{plugins: map[string]*ReportProcessorPlugin{}}
}

func (s *ReportProcessorStore) Upsert(in ReportProcessorPluginInput) (ReportProcessorPlugin, error) {
	name := strings.TrimSpace(in.Name)
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	destination := strings.TrimSpace(in.Destination)
	if name == "" || kind == "" || destination == "" {
		return ReportProcessorPlugin{}, errors.New("name, kind, and destination are required")
	}
	switch kind {
	case "webhook", "queue", "objectstore", "plugin":
	default:
		return ReportProcessorPlugin{}, errors.New("kind must be webhook, queue, objectstore, or plugin")
	}
	timeout := in.TimeoutSeconds
	if timeout <= 0 {
		timeout = 10
	}
	retries := in.RetryLimit
	if retries < 0 {
		retries = 0
	}
	redact := normalizeStringList(in.RedactFields)
	now := time.Now().UTC()
	item := ReportProcessorPlugin{
		Name:           name,
		Kind:           kind,
		Destination:    destination,
		TimeoutSeconds: timeout,
		RetryLimit:     retries,
		RedactFields:   redact,
		Enabled:        in.Enabled,
		UpdatedAt:      now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.plugins {
		if strings.EqualFold(existing.Name, item.Name) {
			item.ID = existing.ID
			item.CreatedAt = existing.CreatedAt
			s.plugins[item.ID] = &item
			return item, nil
		}
	}
	s.nextID++
	item.ID = "report-processor-" + itoa(s.nextID)
	item.CreatedAt = now
	s.plugins[item.ID] = &item
	return item, nil
}

func (s *ReportProcessorStore) List() []ReportProcessorPlugin {
	s.mu.RLock()
	out := make([]ReportProcessorPlugin, 0, len(s.plugins))
	for _, item := range s.plugins {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *ReportProcessorStore) Get(id string) (ReportProcessorPlugin, bool) {
	s.mu.RLock()
	item, ok := s.plugins[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return ReportProcessorPlugin{}, false
	}
	return *item, true
}

func (s *ReportProcessorStore) SetEnabled(id string, enabled bool) (ReportProcessorPlugin, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.plugins[strings.TrimSpace(id)]
	if !ok {
		return ReportProcessorPlugin{}, errors.New("report processor not found")
	}
	item.Enabled = enabled
	item.UpdatedAt = time.Now().UTC()
	return *item, nil
}

func (s *ReportProcessorStore) Dispatch(in ReportProcessorDispatchInput) ReportProcessorDispatchResult {
	runID := strings.TrimSpace(in.RunID)
	if runID == "" {
		return ReportProcessorDispatchResult{
			RunID:         runID,
			Dispatched:    false,
			CheckedAt:     time.Now().UTC(),
			BlockedReason: "run_id is required",
		}
	}
	s.mu.RLock()
	targets := make([]ReportProcessorPlugin, 0, len(s.plugins))
	useIDs := normalizeStringList(in.ProcessorIDs)
	useSet := map[string]struct{}{}
	for _, id := range useIDs {
		useSet[id] = struct{}{}
	}
	for _, plugin := range s.plugins {
		if len(useSet) > 0 {
			if _, ok := useSet[plugin.ID]; !ok {
				continue
			}
		}
		targets = append(targets, *plugin)
	}
	s.mu.RUnlock()
	sort.Slice(targets, func(i, j int) bool { return targets[i].Name < targets[j].Name })

	outcomes := make([]ReportProcessorDispatchOutcome, 0, len(targets))
	acceptedCount := 0
	for _, target := range targets {
		preview := redactReportPayload(in.Payload, target.RedactFields)
		outcome := ReportProcessorDispatchOutcome{
			ProcessorID:   target.ID,
			ProcessorName: target.Name,
			Kind:          target.Kind,
			Preview:       preview,
		}
		if !target.Enabled {
			outcome.Accepted = false
			outcome.Reason = "processor is disabled"
		} else {
			outcome.Accepted = true
			outcome.Reason = "report accepted for post-run processing"
			outcome.DeliveredAt = time.Now().UTC()
			acceptedCount++
		}
		outcomes = append(outcomes, outcome)
	}
	dispatched := acceptedCount > 0
	blocked := ""
	if !dispatched {
		blocked = "no enabled report processors accepted the dispatch"
	}
	return ReportProcessorDispatchResult{
		RunID:         runID,
		Status:        strings.TrimSpace(in.Status),
		Severity:      strings.TrimSpace(in.Severity),
		Dispatched:    dispatched,
		Outcomes:      outcomes,
		CheckedAt:     time.Now().UTC(),
		BlockedReason: blocked,
	}
}

func redactReportPayload(in map[string]any, fields []string) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := map[string]any{}
	redactSet := map[string]struct{}{}
	for _, f := range fields {
		redactSet[strings.ToLower(strings.TrimSpace(f))] = struct{}{}
	}
	for k, v := range in {
		if _, ok := redactSet[strings.ToLower(strings.TrimSpace(k))]; ok {
			out[k] = "***redacted***"
			continue
		}
		out[k] = v
	}
	return out
}
