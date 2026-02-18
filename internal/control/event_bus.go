package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type EventBusKind string

const (
	EventBusWebhook EventBusKind = "webhook"
	EventBusKafka   EventBusKind = "kafka"
	EventBusNATS    EventBusKind = "nats"
)

type EventBusTarget struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Kind      EventBusKind      `json:"kind"`
	URL       string            `json:"url,omitempty"`
	Topic     string            `json:"topic,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Enabled   bool              `json:"enabled"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type EventBusDelivery struct {
	ID        string       `json:"id"`
	Time      time.Time    `json:"time"`
	TargetID  string       `json:"target_id"`
	Target    string       `json:"target"`
	Kind      EventBusKind `json:"kind"`
	EventType string       `json:"event_type,omitempty"`
	Status    string       `json:"status"` // delivered|queued|failed
	Code      int          `json:"code,omitempty"`
	Error     string       `json:"error,omitempty"`
}

type EventBus struct {
	mu         sync.RWMutex
	nextTarget int64
	nextDeliv  int64
	targets    map[string]EventBusTarget
	deliveries []EventBusDelivery
	client     *http.Client
}

func NewEventBus() *EventBus {
	return &EventBus{
		targets: map[string]EventBusTarget{},
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (b *EventBus) Register(in EventBusTarget) (EventBusTarget, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return EventBusTarget{}, errors.New("name is required")
	}
	kind := normalizeEventBusKind(in.Kind)
	if kind == "" {
		return EventBusTarget{}, errors.New("kind must be webhook, kafka, or nats")
	}
	u := strings.TrimSpace(in.URL)
	if u != "" {
		parsed, err := url.Parse(u)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return EventBusTarget{}, errors.New("url must be a valid http or https URL")
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextTarget++
	now := time.Now().UTC()
	item := EventBusTarget{
		ID:        "bus-" + itoa(b.nextTarget),
		Name:      name,
		Kind:      kind,
		URL:       u,
		Topic:     strings.TrimSpace(in.Topic),
		Headers:   cloneStringMap(in.Headers),
		Enabled:   in.Enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}
	b.targets[item.ID] = item
	return cloneEventBusTarget(item), nil
}

func (b *EventBus) ListTargets() []EventBusTarget {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]EventBusTarget, 0, len(b.targets))
	for _, t := range b.targets {
		out = append(out, cloneEventBusTarget(t))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (b *EventBus) SetEnabled(id string, enabled bool) (EventBusTarget, error) {
	id = strings.TrimSpace(id)
	b.mu.Lock()
	defer b.mu.Unlock()
	item, ok := b.targets[id]
	if !ok {
		return EventBusTarget{}, errors.New("event bus target not found")
	}
	item.Enabled = enabled
	item.UpdatedAt = time.Now().UTC()
	b.targets[id] = item
	return cloneEventBusTarget(item), nil
}

func (b *EventBus) Deliveries(limit int) []EventBusDelivery {
	if limit <= 0 {
		limit = 200
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	n := len(b.deliveries)
	if n > limit {
		n = limit
	}
	out := make([]EventBusDelivery, 0, n)
	for i := len(b.deliveries) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, b.deliveries[i])
	}
	return out
}

func (b *EventBus) Publish(event Event) []EventBusDelivery {
	b.mu.RLock()
	targets := make([]EventBusTarget, 0, len(b.targets))
	for _, t := range b.targets {
		if t.Enabled {
			targets = append(targets, t)
		}
	}
	b.mu.RUnlock()

	deliveries := make([]EventBusDelivery, 0, len(targets))
	for _, target := range targets {
		d := b.dispatch(target, event)
		deliveries = append(deliveries, d)
		b.recordDelivery(d)
	}
	return deliveries
}

func (b *EventBus) dispatch(target EventBusTarget, event Event) EventBusDelivery {
	base := EventBusDelivery{
		Time:      time.Now().UTC(),
		TargetID:  target.ID,
		Target:    target.Name,
		Kind:      target.Kind,
		EventType: event.Type,
	}

	payload := map[string]any{
		"event": event,
		"meta": map[string]any{
			"target_kind": target.Kind,
			"topic":       target.Topic,
		},
	}
	body, _ := json.Marshal(payload)
	if strings.TrimSpace(target.URL) == "" {
		if target.Kind == EventBusKafka || target.Kind == EventBusNATS {
			base.Status = "queued"
			return base
		}
		base.Status = "failed"
		base.Error = "url is required for webhook targets"
		return base
	}

	req, err := http.NewRequest(http.MethodPost, target.URL, bytes.NewReader(body))
	if err != nil {
		base.Status = "failed"
		base.Error = err.Error()
		return base
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Masterchef-EventBus-Kind", string(target.Kind))
	req.Header.Set("X-Masterchef-Event-Type", event.Type)
	for k, v := range target.Headers {
		req.Header.Set(k, v)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		base.Status = "failed"
		base.Error = err.Error()
		return base
	}
	defer resp.Body.Close()
	base.Code = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		base.Status = "failed"
		base.Error = "non-2xx status"
		return base
	}
	base.Status = "delivered"
	return base
}

func (b *EventBus) recordDelivery(in EventBusDelivery) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextDeliv++
	in.ID = "bus-delivery-" + itoa(b.nextDeliv)
	b.deliveries = append(b.deliveries, in)
	if len(b.deliveries) > 5000 {
		b.deliveries = append([]EventBusDelivery{}, b.deliveries[len(b.deliveries)-5000:]...)
	}
}

func normalizeEventBusKind(kind EventBusKind) EventBusKind {
	switch strings.ToLower(strings.TrimSpace(string(kind))) {
	case string(EventBusWebhook):
		return EventBusWebhook
	case string(EventBusKafka):
		return EventBusKafka
	case string(EventBusNATS):
		return EventBusNATS
	default:
		return ""
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneEventBusTarget(in EventBusTarget) EventBusTarget {
	out := in
	out.Headers = cloneStringMap(in.Headers)
	return out
}
