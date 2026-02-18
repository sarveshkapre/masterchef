package control

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"
)

type FactRecord struct {
	Node      string         `json:"node"`
	Facts     map[string]any `json:"facts"`
	UpdatedAt time.Time      `json:"updated_at"`
	ExpiresAt time.Time      `json:"expires_at"`
}

type FactCacheQuery struct {
	Field    string `json:"field,omitempty"`
	Equals   string `json:"equals,omitempty"`
	Contains string `json:"contains,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type FactCache struct {
	mu         sync.RWMutex
	defaultTTL time.Duration
	items      map[string]FactRecord
}

func NewFactCache(defaultTTL time.Duration) *FactCache {
	if defaultTTL <= 0 {
		defaultTTL = 5 * time.Minute
	}
	return &FactCache{
		defaultTTL: defaultTTL,
		items:      map[string]FactRecord{},
	}
}

func (c *FactCache) Upsert(node string, facts map[string]any, ttl time.Duration) FactRecord {
	node = normalizeFactNode(node)
	if facts == nil {
		facts = map[string]any{}
	}
	if ttl <= 0 {
		ttl = c.defaultTTL
	}
	now := time.Now().UTC()
	item := FactRecord{
		Node:      node,
		Facts:     cloneFactMap(facts),
		UpdatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[node] = item
	return cloneFactRecord(item)
}

func (c *FactCache) Get(node string) (FactRecord, bool) {
	node = normalizeFactNode(node)
	c.mu.RLock()
	item, ok := c.items[node]
	c.mu.RUnlock()
	if !ok {
		return FactRecord{}, false
	}
	if item.ExpiresAt.Before(time.Now().UTC()) {
		c.mu.Lock()
		delete(c.items, node)
		c.mu.Unlock()
		return FactRecord{}, false
	}
	return cloneFactRecord(item), true
}

func (c *FactCache) Delete(node string) bool {
	node = normalizeFactNode(node)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[node]; !ok {
		return false
	}
	delete(c.items, node)
	return true
}

func (c *FactCache) List() []FactRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now().UTC()
	out := make([]FactRecord, 0, len(c.items))
	for node, item := range c.items {
		if item.ExpiresAt.Before(now) {
			delete(c.items, node)
			continue
		}
		out = append(out, cloneFactRecord(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out
}

func (c *FactCache) Query(q FactCacheQuery) []FactRecord {
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	field := strings.TrimSpace(q.Field)
	equals := strings.TrimSpace(q.Equals)
	contains := strings.ToLower(strings.TrimSpace(q.Contains))

	items := c.List()
	out := make([]FactRecord, 0, minInt(limit, len(items)))
	for _, item := range items {
		if field == "" && equals == "" && contains == "" {
			out = append(out, item)
		} else {
			val, ok := lookupFactField(item.Facts, field)
			if !ok {
				continue
			}
			value := strings.TrimSpace(factValueString(val))
			if equals != "" && value != equals {
				continue
			}
			if contains != "" && !strings.Contains(strings.ToLower(value), contains) {
				continue
			}
			out = append(out, item)
		}
		if len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeFactNode(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func cloneFactMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	buf, _ := json.Marshal(in)
	var out map[string]any
	_ = json.Unmarshal(buf, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func cloneFactRecord(in FactRecord) FactRecord {
	out := in
	out.Facts = cloneFactMap(in.Facts)
	return out
}

func lookupFactField(data map[string]any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return data, true
	}
	cur := any(data)
	for _, part := range strings.Split(path, ".") {
		node, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := node[strings.TrimSpace(part)]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func factValueString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		buf, _ := json.Marshal(t)
		return string(buf)
	}
}
