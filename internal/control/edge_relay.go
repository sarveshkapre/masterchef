package control

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type EdgeRelaySiteInput struct {
	SiteID            string `json:"site_id"`
	Region            string `json:"region"`
	Mode              string `json:"mode"`
	MaxQueueDepth     int    `json:"max_queue_depth,omitempty"`
	HeartbeatInterval int    `json:"heartbeat_interval_seconds,omitempty"`
}

type EdgeRelaySite struct {
	SiteID            string    `json:"site_id"`
	Region            string    `json:"region"`
	Mode              string    `json:"mode"`
	MaxQueueDepth     int       `json:"max_queue_depth"`
	HeartbeatInterval int       `json:"heartbeat_interval_seconds"`
	Connected         bool      `json:"connected"`
	QueueDepth        int       `json:"queue_depth"`
	LastSeenAt        time.Time `json:"last_seen_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type EdgeRelayMessageInput struct {
	SiteID     string `json:"site_id"`
	Direction  string `json:"direction"`
	Payload    string `json:"payload"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
}

type EdgeRelayMessage struct {
	ID          string     `json:"id"`
	SiteID      string     `json:"site_id"`
	Direction   string     `json:"direction"`
	Checksum    string     `json:"checksum"`
	SizeBytes   int        `json:"size_bytes"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
}

type EdgeRelayDeliveryResult struct {
	SiteID         string `json:"site_id"`
	RequestedLimit int    `json:"requested_limit"`
	Delivered      int    `json:"delivered"`
	Remaining      int    `json:"remaining"`
}

type EdgeRelayStore struct {
	mu       sync.RWMutex
	nextID   int64
	sites    map[string]*EdgeRelaySite
	messages map[string]*EdgeRelayMessage
}

func NewEdgeRelayStore() *EdgeRelayStore {
	return &EdgeRelayStore{
		sites:    map[string]*EdgeRelaySite{},
		messages: map[string]*EdgeRelayMessage{},
	}
}

func (s *EdgeRelayStore) UpsertSite(in EdgeRelaySiteInput) (EdgeRelaySite, error) {
	siteID := strings.TrimSpace(in.SiteID)
	if siteID == "" {
		return EdgeRelaySite{}, errors.New("site_id is required")
	}
	mode := normalizeRelayMode(in.Mode)
	if mode == "" {
		return EdgeRelaySite{}, errors.New("mode must be store_and_forward or passthrough")
	}
	maxQueue := in.MaxQueueDepth
	if maxQueue <= 0 {
		maxQueue = 1000
	}
	hb := in.HeartbeatInterval
	if hb <= 0 {
		hb = 60
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.sites[siteID]
	if !ok {
		item = &EdgeRelaySite{SiteID: siteID, Connected: true, LastSeenAt: now}
		s.sites[siteID] = item
	}
	item.Region = strings.TrimSpace(in.Region)
	item.Mode = mode
	item.MaxQueueDepth = maxQueue
	item.HeartbeatInterval = hb
	item.UpdatedAt = now
	if item.Region == "" {
		item.Region = "global"
	}
	item.QueueDepth = s.queueDepthLocked(siteID)
	return cloneRelaySite(*item), nil
}

func (s *EdgeRelayStore) Heartbeat(siteID string) (EdgeRelaySite, error) {
	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		return EdgeRelaySite{}, errors.New("site_id is required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.sites[siteID]
	if !ok {
		return EdgeRelaySite{}, errors.New("site not found")
	}
	item.Connected = true
	item.LastSeenAt = now
	item.UpdatedAt = now
	item.QueueDepth = s.queueDepthLocked(siteID)
	return cloneRelaySite(*item), nil
}

func (s *EdgeRelayStore) GetSite(siteID string) (EdgeRelaySite, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.sites[strings.TrimSpace(siteID)]
	if !ok {
		return EdgeRelaySite{}, false
	}
	cp := cloneRelaySite(*item)
	cp.QueueDepth = s.queueDepthLocked(cp.SiteID)
	return cp, true
}

func (s *EdgeRelayStore) ListSites() []EdgeRelaySite {
	s.mu.RLock()
	out := make([]EdgeRelaySite, 0, len(s.sites))
	for _, item := range s.sites {
		cp := cloneRelaySite(*item)
		cp.QueueDepth = s.queueDepthLocked(cp.SiteID)
		out = append(out, cp)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Region == out[j].Region {
			return out[i].SiteID < out[j].SiteID
		}
		return out[i].Region < out[j].Region
	})
	return out
}

func (s *EdgeRelayStore) QueueMessage(in EdgeRelayMessageInput) (EdgeRelayMessage, error) {
	siteID := strings.TrimSpace(in.SiteID)
	direction := strings.ToLower(strings.TrimSpace(in.Direction))
	payload := in.Payload
	if siteID == "" || direction == "" {
		return EdgeRelayMessage{}, errors.New("site_id and direction are required")
	}
	if direction != "ingress" && direction != "egress" {
		return EdgeRelayMessage{}, errors.New("direction must be ingress or egress")
	}
	now := time.Now().UTC()
	ttl := in.TTLSeconds
	if ttl <= 0 {
		ttl = 3600
	}
	sum := sha256.Sum256([]byte(siteID + "|" + direction + "|" + payload))
	checksum := "sha256:" + hex.EncodeToString(sum[:])

	s.mu.Lock()
	defer s.mu.Unlock()
	site, ok := s.sites[siteID]
	if !ok {
		return EdgeRelayMessage{}, errors.New("site not found")
	}
	if site.MaxQueueDepth > 0 && s.queueDepthLocked(siteID) >= site.MaxQueueDepth {
		return EdgeRelayMessage{}, errors.New("relay queue is full for site")
	}
	s.nextID++
	item := EdgeRelayMessage{
		ID:        "relay-msg-" + itoa(s.nextID),
		SiteID:    siteID,
		Direction: direction,
		Checksum:  checksum,
		SizeBytes: len(payload),
		Status:    "queued",
		CreatedAt: now,
		ExpiresAt: now.Add(time.Duration(ttl) * time.Second),
	}
	s.messages[item.ID] = &item
	site.QueueDepth = s.queueDepthLocked(siteID)
	site.UpdatedAt = now
	return cloneRelayMessage(item), nil
}

func (s *EdgeRelayStore) Deliver(siteID string, limit int) (EdgeRelayDeliveryResult, error) {
	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		return EdgeRelayDeliveryResult{}, errors.New("site_id is required")
	}
	if limit <= 0 {
		limit = 100
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	site, ok := s.sites[siteID]
	if !ok {
		return EdgeRelayDeliveryResult{}, errors.New("site not found")
	}
	s.expireLocked(now)
	delivered := 0
	for _, msg := range s.messages {
		if msg.SiteID != siteID || msg.Status != "queued" {
			continue
		}
		if delivered >= limit {
			break
		}
		msg.Status = "delivered"
		ts := now
		msg.DeliveredAt = &ts
		delivered++
	}
	site.QueueDepth = s.queueDepthLocked(siteID)
	site.UpdatedAt = now
	return EdgeRelayDeliveryResult{
		SiteID:         siteID,
		RequestedLimit: limit,
		Delivered:      delivered,
		Remaining:      site.QueueDepth,
	}, nil
}

func (s *EdgeRelayStore) ListMessages(siteID string, limit int) []EdgeRelayMessage {
	siteID = strings.TrimSpace(siteID)
	now := time.Now().UTC()
	s.mu.Lock()
	s.expireLocked(now)
	out := make([]EdgeRelayMessage, 0, len(s.messages))
	for _, item := range s.messages {
		if siteID != "" && item.SiteID != siteID {
			continue
		}
		out = append(out, cloneRelayMessage(*item))
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *EdgeRelayStore) queueDepthLocked(siteID string) int {
	depth := 0
	for _, msg := range s.messages {
		if msg.SiteID == siteID && msg.Status == "queued" {
			depth++
		}
	}
	return depth
}

func (s *EdgeRelayStore) expireLocked(now time.Time) {
	for id, msg := range s.messages {
		if now.After(msg.ExpiresAt) || now.Equal(msg.ExpiresAt) {
			delete(s.messages, id)
		}
	}
}

func normalizeRelayMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "store_and_forward", "passthrough":
		return mode
	default:
		return ""
	}
}

func cloneRelaySite(in EdgeRelaySite) EdgeRelaySite {
	return in
}

func cloneRelayMessage(in EdgeRelayMessage) EdgeRelayMessage {
	out := in
	if in.DeliveredAt != nil {
		ts := *in.DeliveredAt
		out.DeliveredAt = &ts
	}
	return out
}
