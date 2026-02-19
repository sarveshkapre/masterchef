package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type MultiMasterNodeInput struct {
	NodeID  string `json:"node_id"`
	Region  string `json:"region"`
	Address string `json:"address"`
	Role    string `json:"role"`
	Status  string `json:"status"`
}

type MultiMasterNode struct {
	NodeID        string    `json:"node_id"`
	Region        string    `json:"region"`
	Address       string    `json:"address"`
	Role          string    `json:"role"`
	Status        string    `json:"status"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type MultiMasterCacheEntry struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	RefID     string         `json:"ref_id"`
	Payload   map[string]any `json:"payload"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type MultiMasterSyncResult struct {
	GeneratedAt   time.Time `json:"generated_at"`
	SyncedJobs    int       `json:"synced_jobs"`
	SyncedEvents  int       `json:"synced_events"`
	TotalEntries  int       `json:"total_entries"`
	PrunedEntries int       `json:"pruned_entries"`
}

type MultiMasterStore struct {
	mu      sync.RWMutex
	nodes   map[string]*MultiMasterNode
	entries map[string]*MultiMasterCacheEntry
}

func NewMultiMasterStore() *MultiMasterStore {
	return &MultiMasterStore{
		nodes:   map[string]*MultiMasterNode{},
		entries: map[string]*MultiMasterCacheEntry{},
	}
}

func (s *MultiMasterStore) UpsertNode(in MultiMasterNodeInput) (MultiMasterNode, error) {
	nodeID := strings.TrimSpace(in.NodeID)
	if nodeID == "" {
		return MultiMasterNode{}, errors.New("node_id is required")
	}
	region := strings.TrimSpace(in.Region)
	if region == "" {
		region = "global"
	}
	role := strings.ToLower(strings.TrimSpace(in.Role))
	if role == "" {
		role = "secondary"
	}
	status := normalizeMasterNodeStatus(in.Status)
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.nodes[nodeID]
	if !ok {
		item = &MultiMasterNode{NodeID: nodeID}
		s.nodes[nodeID] = item
	}
	item.Region = region
	item.Address = strings.TrimSpace(in.Address)
	item.Role = role
	item.Status = status
	item.UpdatedAt = now
	if item.LastHeartbeat.IsZero() {
		item.LastHeartbeat = now
	}
	return cloneMultiMasterNode(*item), nil
}

func (s *MultiMasterStore) Heartbeat(nodeID, status string) (MultiMasterNode, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return MultiMasterNode{}, errors.New("node_id is required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.nodes[nodeID]
	if !ok {
		return MultiMasterNode{}, errors.New("node not found")
	}
	if status != "" {
		item.Status = normalizeMasterNodeStatus(status)
	}
	item.LastHeartbeat = now
	item.UpdatedAt = now
	return cloneMultiMasterNode(*item), nil
}

func (s *MultiMasterStore) GetNode(nodeID string) (MultiMasterNode, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.nodes[strings.TrimSpace(nodeID)]
	if !ok {
		return MultiMasterNode{}, false
	}
	return cloneMultiMasterNode(*item), true
}

func (s *MultiMasterStore) ListNodes() []MultiMasterNode {
	s.mu.RLock()
	out := make([]MultiMasterNode, 0, len(s.nodes))
	for _, item := range s.nodes {
		out = append(out, cloneMultiMasterNode(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Region == out[j].Region {
			return out[i].NodeID < out[j].NodeID
		}
		return out[i].Region < out[j].Region
	})
	return out
}

func (s *MultiMasterStore) SyncCentralCache(jobs []Job, events []Event, maxEntries int) MultiMasterSyncResult {
	if maxEntries <= 0 {
		maxEntries = 5000
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, job := range jobs {
		id := "job:" + strings.TrimSpace(job.ID)
		s.entries[id] = &MultiMasterCacheEntry{
			ID:    id,
			Kind:  "job",
			RefID: job.ID,
			Payload: map[string]any{
				"id":          job.ID,
				"config_path": job.ConfigPath,
				"priority":    job.Priority,
				"status":      string(job.Status),
				"created_at":  job.CreatedAt,
				"started_at":  job.StartedAt,
				"ended_at":    job.EndedAt,
			},
			UpdatedAt: now,
		}
	}
	for _, event := range events {
		id := "event:" + strings.TrimSpace(event.Type) + ":" + event.Time.Format(time.RFC3339Nano)
		s.entries[id] = &MultiMasterCacheEntry{
			ID:    id,
			Kind:  "event",
			RefID: event.Type,
			Payload: map[string]any{
				"time":    event.Time,
				"type":    event.Type,
				"message": event.Message,
				"fields":  cloneAnyMap(event.Fields),
			},
			UpdatedAt: now,
		}
	}

	pruned := 0
	if len(s.entries) > maxEntries {
		items := make([]*MultiMasterCacheEntry, 0, len(s.entries))
		for _, item := range s.entries {
			items = append(items, item)
		}
		sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
		keep := map[string]struct{}{}
		for i := 0; i < maxEntries && i < len(items); i++ {
			keep[items[i].ID] = struct{}{}
		}
		for id := range s.entries {
			if _, ok := keep[id]; !ok {
				delete(s.entries, id)
				pruned++
			}
		}
	}

	return MultiMasterSyncResult{
		GeneratedAt:   now,
		SyncedJobs:    len(jobs),
		SyncedEvents:  len(events),
		TotalEntries:  len(s.entries),
		PrunedEntries: pruned,
	}
}

func (s *MultiMasterStore) ListCentralCache(kind string, limit int) []MultiMasterCacheEntry {
	kind = strings.ToLower(strings.TrimSpace(kind))
	s.mu.RLock()
	out := make([]MultiMasterCacheEntry, 0, len(s.entries))
	for _, item := range s.entries {
		if kind != "" && item.Kind != kind {
			continue
		}
		out = append(out, cloneMultiMasterEntry(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func normalizeMasterNodeStatus(status string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "active", "degraded", "offline":
		return s
	default:
		return "active"
	}
}

func cloneMultiMasterNode(in MultiMasterNode) MultiMasterNode {
	return in
}

func cloneMultiMasterEntry(in MultiMasterCacheEntry) MultiMasterCacheEntry {
	out := in
	out.Payload = cloneAnyMap(in.Payload)
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
