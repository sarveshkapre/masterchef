package control

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type WebhookSubscription struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	URL          string    `json:"url"`
	EventPrefix  string    `json:"event_prefix"`
	Enabled      bool      `json:"enabled"`
	Secret       string    `json:"secret,omitempty"`
	SuccessCount int64     `json:"success_count"`
	FailureCount int64     `json:"failure_count"`
	LastError    string    `json:"last_error,omitempty"`
	LastDelivery time.Time `json:"last_delivery,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type WebhookDelivery struct {
	ID          string    `json:"id"`
	WebhookID   string    `json:"webhook_id"`
	EventType   string    `json:"event_type"`
	Status      string    `json:"status"` // delivered|failed
	StatusCode  int       `json:"status_code,omitempty"`
	Error       string    `json:"error,omitempty"`
	DeliveredAt time.Time `json:"delivered_at"`
}

type WebhookDispatcher struct {
	mu          sync.RWMutex
	nextID      int64
	nextDelID   int64
	webhooks    map[string]*WebhookSubscription
	deliveries  []WebhookDelivery
	deliveryCap int
	client      *http.Client
}

func NewWebhookDispatcher(limit int) *WebhookDispatcher {
	if limit <= 0 {
		limit = 5000
	}
	return &WebhookDispatcher{
		webhooks:    map[string]*WebhookSubscription{},
		deliveries:  make([]WebhookDelivery, 0, limit),
		deliveryCap: limit,
		client: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

func (d *WebhookDispatcher) Register(in WebhookSubscription) (WebhookSubscription, error) {
	if strings.TrimSpace(in.Name) == "" {
		return WebhookSubscription{}, errors.New("webhook name is required")
	}
	if strings.TrimSpace(in.URL) == "" {
		return WebhookSubscription{}, errors.New("webhook url is required")
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(in.URL)), "http://") && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(in.URL)), "https://") {
		return WebhookSubscription{}, errors.New("webhook url must be http or https")
	}
	if strings.TrimSpace(in.EventPrefix) == "" {
		return WebhookSubscription{}, errors.New("event_prefix is required")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.nextID++
	now := time.Now().UTC()
	in.ID = "wh-" + itoa(d.nextID)
	if !in.Enabled {
		in.Enabled = true
	}
	in.CreatedAt = now
	in.UpdatedAt = now
	cp := in
	d.webhooks[in.ID] = &cp
	return cp, nil
}

func (d *WebhookDispatcher) List() []WebhookSubscription {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]WebhookSubscription, 0, len(d.webhooks))
	for _, w := range d.webhooks {
		out = append(out, cloneWebhook(*w))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (d *WebhookDispatcher) Get(id string) (WebhookSubscription, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	w, ok := d.webhooks[id]
	if !ok {
		return WebhookSubscription{}, errors.New("webhook not found")
	}
	return cloneWebhook(*w), nil
}

func (d *WebhookDispatcher) SetEnabled(id string, enabled bool) (WebhookSubscription, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	w, ok := d.webhooks[id]
	if !ok {
		return WebhookSubscription{}, errors.New("webhook not found")
	}
	w.Enabled = enabled
	w.UpdatedAt = time.Now().UTC()
	return cloneWebhook(*w), nil
}

func (d *WebhookDispatcher) Dispatch(event Event) []WebhookDelivery {
	d.mu.RLock()
	subs := make([]WebhookSubscription, 0, len(d.webhooks))
	for _, wh := range d.webhooks {
		subs = append(subs, cloneWebhook(*wh))
	}
	d.mu.RUnlock()

	payload, _ := json.Marshal(event)
	delivered := make([]WebhookDelivery, 0)
	for _, sub := range subs {
		if !sub.Enabled {
			continue
		}
		if !strings.HasPrefix(event.Type, sub.EventPrefix) {
			continue
		}
		req, err := http.NewRequest(http.MethodPost, sub.URL, bytes.NewReader(payload))
		if err != nil {
			delivered = append(delivered, d.recordDelivery(sub.ID, event.Type, 0, err))
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Masterchef-Event-Type", event.Type)
		if strings.TrimSpace(sub.Secret) != "" {
			req.Header.Set("X-Masterchef-Signature", signPayload(payload, sub.Secret))
		}

		resp, err := d.client.Do(req)
		if err != nil {
			delivered = append(delivered, d.recordDelivery(sub.ID, event.Type, 0, err))
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			delivered = append(delivered, d.recordDelivery(sub.ID, event.Type, resp.StatusCode, errors.New("non-2xx status")))
			continue
		}
		delivered = append(delivered, d.recordDelivery(sub.ID, event.Type, resp.StatusCode, nil))
	}
	return delivered
}

func (d *WebhookDispatcher) recordDelivery(webhookID, eventType string, statusCode int, err error) WebhookDelivery {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.nextDelID++
	now := time.Now().UTC()
	delivery := WebhookDelivery{
		ID:          "whdel-" + itoa(d.nextDelID),
		WebhookID:   webhookID,
		EventType:   eventType,
		StatusCode:  statusCode,
		DeliveredAt: now,
	}
	if err != nil {
		delivery.Status = "failed"
		delivery.Error = err.Error()
		if wh, ok := d.webhooks[webhookID]; ok {
			wh.FailureCount++
			wh.LastError = err.Error()
			wh.LastDelivery = now
			wh.UpdatedAt = now
		}
	} else {
		delivery.Status = "delivered"
		if wh, ok := d.webhooks[webhookID]; ok {
			wh.SuccessCount++
			wh.LastError = ""
			wh.LastDelivery = now
			wh.UpdatedAt = now
		}
	}
	if len(d.deliveries) >= d.deliveryCap {
		copy(d.deliveries[0:], d.deliveries[1:])
		d.deliveries[len(d.deliveries)-1] = delivery
	} else {
		d.deliveries = append(d.deliveries, delivery)
	}
	return delivery
}

func (d *WebhookDispatcher) Deliveries(limit int) []WebhookDelivery {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if limit <= 0 {
		limit = 200
	}
	if len(d.deliveries) == 0 {
		return []WebhookDelivery{}
	}
	start := len(d.deliveries) - limit
	if start < 0 {
		start = 0
	}
	out := make([]WebhookDelivery, len(d.deliveries[start:]))
	copy(out, d.deliveries[start:])
	return out
}

func signPayload(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write(payload)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

func cloneWebhook(in WebhookSubscription) WebhookSubscription {
	out := in
	return out
}
