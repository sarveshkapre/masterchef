package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type AlertStatus string

const (
	AlertOpen         AlertStatus = "open"
	AlertAcknowledged AlertStatus = "acknowledged"
	AlertResolved     AlertStatus = "resolved"
)

type AlertSuppression struct {
	Fingerprint string    `json:"fingerprint"`
	Until       time.Time `json:"until"`
	Reason      string    `json:"reason,omitempty"`
	Hits        int       `json:"hits"`
}

type AlertItem struct {
	ID              string         `json:"id"`
	Fingerprint     string         `json:"fingerprint"`
	EventType       string         `json:"event_type"`
	Message         string         `json:"message"`
	Severity        string         `json:"severity"`
	Route           string         `json:"route"`
	Count           int            `json:"count"`
	SuppressedCount int            `json:"suppressed_count"`
	FirstSeenAt     time.Time      `json:"first_seen_at"`
	LastSeenAt      time.Time      `json:"last_seen_at"`
	Status          AlertStatus    `json:"status"`
	Fields          map[string]any `json:"fields,omitempty"`
}

type AlertIngest struct {
	Fingerprint string
	EventType   string
	Message     string
	Severity    string
	Fields      map[string]any
}

type AlertIngestResult struct {
	Item         AlertItem `json:"item"`
	Created      bool      `json:"created"`
	Deduplicated bool      `json:"deduplicated"`
	Suppressed   bool      `json:"suppressed"`
}

type AlertSummary struct {
	Open               int `json:"open"`
	Acknowledged       int `json:"acknowledged"`
	Resolved           int `json:"resolved"`
	ActiveSuppressions int `json:"active_suppressions"`
	Total              int `json:"total"`
}

type AlertInbox struct {
	mu            sync.RWMutex
	nextID        int64
	items         map[string]*AlertItem
	byFingerprint map[string]string
	suppressions  map[string]AlertSuppression
}

func NewAlertInbox() *AlertInbox {
	return &AlertInbox{
		items:         map[string]*AlertItem{},
		byFingerprint: map[string]string{},
		suppressions:  map[string]AlertSuppression{},
	}
}

func (a *AlertInbox) Ingest(in AlertIngest) AlertIngestResult {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now().UTC()
	a.cleanupSuppressionsLocked(now)

	fp := normalizedAlertFingerprint(in)
	if fp == "" {
		fp = "alert:unknown"
	}
	if sup, ok := a.suppressions[fp]; ok && now.Before(sup.Until) {
		sup.Hits++
		a.suppressions[fp] = sup
		itemID := a.byFingerprint[fp]
		if item, ok := a.items[itemID]; ok {
			item.SuppressedCount++
			item.LastSeenAt = now
			return AlertIngestResult{Item: cloneAlert(*item), Suppressed: true}
		}
		return AlertIngestResult{
			Item: AlertItem{
				Fingerprint: fp,
				EventType:   strings.TrimSpace(in.EventType),
				Message:     defaultAlertMessage(in),
				Severity:    normalizeSeverity(in.Severity),
				Route:       routeForSeverity(normalizeSeverity(in.Severity)),
			},
			Suppressed: true,
		}
	}

	itemID := a.byFingerprint[fp]
	if item, ok := a.items[itemID]; ok {
		item.LastSeenAt = now
		item.Count++
		item.EventType = strings.TrimSpace(in.EventType)
		item.Message = defaultAlertMessage(in)
		item.Fields = copyFields(in.Fields)
		severity := normalizeSeverity(in.Severity)
		item.Severity = chooseMaxSeverity(item.Severity, severity)
		item.Route = routeForSeverity(item.Severity)
		if item.Status != AlertOpen {
			item.Status = AlertOpen
		}
		return AlertIngestResult{
			Item:         cloneAlert(*item),
			Deduplicated: true,
		}
	}

	a.nextID++
	id := "alert-" + itoa(a.nextID)
	severity := normalizeSeverity(in.Severity)
	item := &AlertItem{
		ID:          id,
		Fingerprint: fp,
		EventType:   strings.TrimSpace(in.EventType),
		Message:     defaultAlertMessage(in),
		Severity:    severity,
		Route:       routeForSeverity(severity),
		Count:       1,
		FirstSeenAt: now,
		LastSeenAt:  now,
		Status:      AlertOpen,
		Fields:      copyFields(in.Fields),
	}
	a.items[id] = item
	a.byFingerprint[fp] = id
	return AlertIngestResult{Item: cloneAlert(*item), Created: true}
}

func (a *AlertInbox) IngestEvent(e Event) (AlertIngestResult, bool) {
	severity := inferSeverityFromEvent(e)
	if severity == "" {
		return AlertIngestResult{}, false
	}
	return a.Ingest(AlertIngest{
		EventType: e.Type,
		Message:   e.Message,
		Severity:  severity,
		Fields:    e.Fields,
	}), true
}

func (a *AlertInbox) Acknowledge(id string) (AlertItem, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	item, ok := a.items[strings.TrimSpace(id)]
	if !ok {
		return AlertItem{}, errors.New("alert not found")
	}
	item.Status = AlertAcknowledged
	return cloneAlert(*item), nil
}

func (a *AlertInbox) Resolve(id string) (AlertItem, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	item, ok := a.items[strings.TrimSpace(id)]
	if !ok {
		return AlertItem{}, errors.New("alert not found")
	}
	item.Status = AlertResolved
	return cloneAlert(*item), nil
}

func (a *AlertInbox) Get(id string) (AlertItem, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	item, ok := a.items[strings.TrimSpace(id)]
	if !ok {
		return AlertItem{}, errors.New("alert not found")
	}
	return cloneAlert(*item), nil
}

func (a *AlertInbox) Suppress(fingerprint string, duration time.Duration, reason string) (AlertSuppression, error) {
	fingerprint = strings.TrimSpace(strings.ToLower(fingerprint))
	if fingerprint == "" {
		return AlertSuppression{}, errors.New("fingerprint is required")
	}
	if duration <= 0 {
		return AlertSuppression{}, errors.New("duration must be positive")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now().UTC()
	cur := a.suppressions[fingerprint]
	cur.Fingerprint = fingerprint
	cur.Reason = strings.TrimSpace(reason)
	cur.Until = now.Add(duration)
	a.suppressions[fingerprint] = cur
	return cur, nil
}

func (a *AlertInbox) ClearSuppression(fingerprint string) bool {
	fingerprint = strings.TrimSpace(strings.ToLower(fingerprint))
	if fingerprint == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.suppressions[fingerprint]; !ok {
		return false
	}
	delete(a.suppressions, fingerprint)
	return true
}

func (a *AlertInbox) List(status string, limit int) []AlertItem {
	if limit <= 0 {
		limit = 200
	}
	status = strings.ToLower(strings.TrimSpace(status))
	a.mu.Lock()
	now := time.Now().UTC()
	a.cleanupSuppressionsLocked(now)
	a.mu.Unlock()

	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]AlertItem, 0, len(a.items))
	for _, item := range a.items {
		if status != "" && status != "all" && string(item.Status) != status {
			continue
		}
		out = append(out, cloneAlert(*item))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (a *AlertInbox) Suppressions() []AlertSuppression {
	a.mu.Lock()
	now := time.Now().UTC()
	a.cleanupSuppressionsLocked(now)
	items := make([]AlertSuppression, 0, len(a.suppressions))
	for _, s := range a.suppressions {
		items = append(items, s)
	}
	a.mu.Unlock()
	sort.Slice(items, func(i, j int) bool {
		if items[i].Until.Equal(items[j].Until) {
			return items[i].Fingerprint < items[j].Fingerprint
		}
		return items[i].Until.Before(items[j].Until)
	})
	return items
}

func (a *AlertInbox) Summary() AlertSummary {
	a.mu.Lock()
	now := time.Now().UTC()
	a.cleanupSuppressionsLocked(now)
	a.mu.Unlock()

	a.mu.RLock()
	defer a.mu.RUnlock()
	s := AlertSummary{Total: len(a.items), ActiveSuppressions: len(a.suppressions)}
	for _, item := range a.items {
		switch item.Status {
		case AlertOpen:
			s.Open++
		case AlertAcknowledged:
			s.Acknowledged++
		case AlertResolved:
			s.Resolved++
		}
	}
	return s
}

func (a *AlertInbox) cleanupSuppressionsLocked(now time.Time) {
	for fp, s := range a.suppressions {
		if !now.Before(s.Until) {
			delete(a.suppressions, fp)
		}
	}
}

func normalizedAlertFingerprint(in AlertIngest) string {
	if explicit := strings.TrimSpace(strings.ToLower(in.Fingerprint)); explicit != "" {
		return explicit
	}
	base := strings.TrimSpace(strings.ToLower(in.EventType))
	if base == "" {
		base = "alert"
	}
	parts := []string{base}
	for _, key := range []string{"service", "host", "resource", "component", "sev", "severity"} {
		if v, ok := readStringField(in.Fields, key); ok && v != "" {
			parts = append(parts, key+"="+strings.ToLower(v))
		}
	}
	if msg := strings.TrimSpace(strings.ToLower(in.Message)); msg != "" {
		parts = append(parts, "msg="+msg)
	}
	return strings.Join(parts, "|")
}

func inferSeverityFromEvent(e Event) string {
	if sev, ok := readStringField(e.Fields, "severity"); ok && sev != "" {
		return normalizeSeverity(sev)
	}
	if sev, ok := readStringField(e.Fields, "sev"); ok && sev != "" {
		return normalizeSeverity(sev)
	}
	typ := strings.ToLower(strings.TrimSpace(e.Type))
	switch {
	case strings.Contains(typ, "critical"):
		return "critical"
	case strings.Contains(typ, "alert"):
		return "high"
	case strings.Contains(typ, "error"):
		return "high"
	case strings.Contains(typ, "saturation"):
		return "medium"
	default:
		return ""
	}
}

func normalizeSeverity(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "critical", "crit", "p0":
		return "critical"
	case "high", "error", "p1":
		return "high"
	case "medium", "warn", "warning", "p2":
		return "medium"
	case "low", "info", "p3":
		return "low"
	default:
		return "medium"
	}
}

func routeForSeverity(severity string) string {
	switch normalizeSeverity(severity) {
	case "critical":
		return "pager"
	case "high":
		return "ticket"
	case "medium":
		return "chatops"
	default:
		return "digest"
	}
}

func severityRank(severity string) int {
	switch normalizeSeverity(severity) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	default:
		return 1
	}
}

func chooseMaxSeverity(current, candidate string) string {
	if severityRank(candidate) > severityRank(current) {
		return normalizeSeverity(candidate)
	}
	return normalizeSeverity(current)
}

func defaultAlertMessage(in AlertIngest) string {
	if strings.TrimSpace(in.Message) != "" {
		return in.Message
	}
	if strings.TrimSpace(in.EventType) != "" {
		return in.EventType
	}
	return "alert"
}

func copyFields(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func readStringField(fields map[string]any, key string) (string, bool) {
	if len(fields) == 0 {
		return "", false
	}
	v, ok := fields[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(s), true
}

func cloneAlert(in AlertItem) AlertItem {
	in.Fields = copyFields(in.Fields)
	return in
}
