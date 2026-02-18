package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type NotificationTarget struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"` // chatops|incident|ticket
	URL          string    `json:"url"`
	Route        string    `json:"route"` // pager|ticket|chatops|digest|*
	Enabled      bool      `json:"enabled"`
	SuccessCount int64     `json:"success_count"`
	FailureCount int64     `json:"failure_count"`
	LastError    string    `json:"last_error,omitempty"`
	LastDelivery time.Time `json:"last_delivery,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type NotificationDelivery struct {
	ID          string    `json:"id"`
	TargetID    string    `json:"target_id"`
	AlertID     string    `json:"alert_id"`
	AlertRoute  string    `json:"alert_route"`
	Status      string    `json:"status"` // delivered|failed
	StatusCode  int       `json:"status_code,omitempty"`
	Error       string    `json:"error,omitempty"`
	DeliveredAt time.Time `json:"delivered_at"`
}

type NotificationRouter struct {
	mu          sync.RWMutex
	nextID      int64
	nextDelID   int64
	targets     map[string]*NotificationTarget
	deliveries  []NotificationDelivery
	deliveryCap int
	client      *http.Client
}

func NewNotificationRouter(limit int) *NotificationRouter {
	if limit <= 0 {
		limit = 5000
	}
	return &NotificationRouter{
		targets:     map[string]*NotificationTarget{},
		deliveries:  make([]NotificationDelivery, 0, limit),
		deliveryCap: limit,
		client: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

func (r *NotificationRouter) Register(in NotificationTarget) (NotificationTarget, error) {
	if strings.TrimSpace(in.Name) == "" {
		return NotificationTarget{}, errors.New("notification target name is required")
	}
	if strings.TrimSpace(in.URL) == "" {
		return NotificationTarget{}, errors.New("notification target url is required")
	}
	url := strings.ToLower(strings.TrimSpace(in.URL))
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return NotificationTarget{}, errors.New("notification target url must be http or https")
	}
	kind := normalizeNotificationKind(in.Kind)
	if kind == "" {
		return NotificationTarget{}, errors.New("notification kind must be chatops, incident, or ticket")
	}
	route := normalizeNotificationRoute(in.Route)
	if route == "" {
		return NotificationTarget{}, errors.New("notification route must be pager, ticket, chatops, digest, or *")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	now := time.Now().UTC()
	in.ID = "notify-" + itoa(r.nextID)
	in.Kind = kind
	in.Route = route
	if !in.Enabled {
		in.Enabled = true
	}
	in.CreatedAt = now
	in.UpdatedAt = now
	cp := in
	r.targets[in.ID] = &cp
	return cp, nil
}

func (r *NotificationRouter) List() []NotificationTarget {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]NotificationTarget, 0, len(r.targets))
	for _, t := range r.targets {
		out = append(out, cloneNotificationTarget(*t))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *NotificationRouter) SetEnabled(id string, enabled bool) (NotificationTarget, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.targets[id]
	if !ok {
		return NotificationTarget{}, errors.New("notification target not found")
	}
	t.Enabled = enabled
	t.UpdatedAt = time.Now().UTC()
	return cloneNotificationTarget(*t), nil
}

func (r *NotificationRouter) NotifyAlert(alert AlertItem) []NotificationDelivery {
	r.mu.RLock()
	targets := make([]NotificationTarget, 0, len(r.targets))
	for _, t := range r.targets {
		targets = append(targets, cloneNotificationTarget(*t))
	}
	r.mu.RUnlock()

	payload, _ := json.Marshal(map[string]any{
		"type":  "alert.notification",
		"alert": alert,
	})
	deliveries := make([]NotificationDelivery, 0)
	for _, target := range targets {
		if !target.Enabled {
			continue
		}
		if target.Route != "*" && target.Route != alert.Route {
			continue
		}
		req, err := http.NewRequest(http.MethodPost, target.URL, bytes.NewReader(payload))
		if err != nil {
			deliveries = append(deliveries, r.recordDelivery(target.ID, alert.ID, alert.Route, 0, err))
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Masterchef-Notification-Kind", target.Kind)
		req.Header.Set("X-Masterchef-Alert-Route", alert.Route)

		resp, err := r.client.Do(req)
		if err != nil {
			deliveries = append(deliveries, r.recordDelivery(target.ID, alert.ID, alert.Route, 0, err))
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			deliveries = append(deliveries, r.recordDelivery(target.ID, alert.ID, alert.Route, resp.StatusCode, errors.New("non-2xx status")))
			continue
		}
		deliveries = append(deliveries, r.recordDelivery(target.ID, alert.ID, alert.Route, resp.StatusCode, nil))
	}
	return deliveries
}

func (r *NotificationRouter) Deliveries(limit int) []NotificationDelivery {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 {
		limit = 200
	}
	if len(r.deliveries) == 0 {
		return []NotificationDelivery{}
	}
	start := len(r.deliveries) - limit
	if start < 0 {
		start = 0
	}
	out := make([]NotificationDelivery, len(r.deliveries[start:]))
	copy(out, r.deliveries[start:])
	return out
}

func (r *NotificationRouter) recordDelivery(targetID, alertID, alertRoute string, statusCode int, err error) NotificationDelivery {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextDelID++
	now := time.Now().UTC()
	d := NotificationDelivery{
		ID:          "notify-del-" + itoa(r.nextDelID),
		TargetID:    targetID,
		AlertID:     alertID,
		AlertRoute:  alertRoute,
		StatusCode:  statusCode,
		DeliveredAt: now,
	}
	if err != nil {
		d.Status = "failed"
		d.Error = err.Error()
		if t, ok := r.targets[targetID]; ok {
			t.FailureCount++
			t.LastError = err.Error()
			t.LastDelivery = now
			t.UpdatedAt = now
		}
	} else {
		d.Status = "delivered"
		if t, ok := r.targets[targetID]; ok {
			t.SuccessCount++
			t.LastError = ""
			t.LastDelivery = now
			t.UpdatedAt = now
		}
	}
	if len(r.deliveries) >= r.deliveryCap {
		copy(r.deliveries[0:], r.deliveries[1:])
		r.deliveries[len(r.deliveries)-1] = d
	} else {
		r.deliveries = append(r.deliveries, d)
	}
	return d
}

func normalizeNotificationKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "chatops":
		return "chatops"
	case "incident":
		return "incident"
	case "ticket":
		return "ticket"
	default:
		return ""
	}
}

func normalizeNotificationRoute(route string) string {
	switch strings.ToLower(strings.TrimSpace(route)) {
	case "*":
		return "*"
	case "pager":
		return "pager"
	case "ticket":
		return "ticket"
	case "chatops":
		return "chatops"
	case "digest":
		return "digest"
	default:
		return ""
	}
}

func cloneNotificationTarget(in NotificationTarget) NotificationTarget {
	return in
}
