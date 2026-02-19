package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ExportedResourceInput struct {
	Type       string            `json:"type"`
	Host       string            `json:"host,omitempty"`
	ResourceID string            `json:"resource_id,omitempty"`
	Source     string            `json:"source,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type ExportedResource struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Host       string            `json:"host,omitempty"`
	ResourceID string            `json:"resource_id,omitempty"`
	Source     string            `json:"source,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

type CollectorTerm struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type CollectorResult struct {
	Selector string             `json:"selector"`
	Terms    []CollectorTerm    `json:"terms"`
	Count    int                `json:"count"`
	Items    []ExportedResource `json:"items"`
}

type ExportedResourceStore struct {
	mu      sync.RWMutex
	nextID  int64
	max     int
	items   map[string]ExportedResource
	ordered []string
}

func NewExportedResourceStore(max int) *ExportedResourceStore {
	if max <= 0 {
		max = 5000
	}
	return &ExportedResourceStore{
		max:   max,
		items: map[string]ExportedResource{},
	}
}

func (s *ExportedResourceStore) Add(in ExportedResourceInput) (ExportedResource, error) {
	typ := strings.ToLower(strings.TrimSpace(in.Type))
	if typ == "" {
		return ExportedResource{}, errors.New("type is required")
	}
	item := ExportedResource{
		Type:       typ,
		Host:       strings.ToLower(strings.TrimSpace(in.Host)),
		ResourceID: strings.ToLower(strings.TrimSpace(in.ResourceID)),
		Source:     strings.ToLower(strings.TrimSpace(in.Source)),
		Attributes: normalizeStringMap(in.Attributes),
		CreatedAt:  time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item.ID = "xres-" + itoa(s.nextID)
	s.items[item.ID] = cloneExportedResource(item)
	s.ordered = append(s.ordered, item.ID)
	s.trimLocked()
	return item, nil
}

func (s *ExportedResourceStore) List(limit int) []ExportedResource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.ordered) {
		limit = len(s.ordered)
	}
	out := make([]ExportedResource, 0, limit)
	for i := len(s.ordered) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, cloneExportedResource(s.items[s.ordered[i]]))
	}
	return out
}

func (s *ExportedResourceStore) Collect(selector string, limit int) (CollectorResult, error) {
	terms, err := parseCollectorSelector(selector)
	if err != nil {
		return CollectorResult{}, err
	}
	s.mu.RLock()
	items := make([]ExportedResource, 0, len(s.items))
	for _, id := range s.ordered {
		items = append(items, cloneExportedResource(s.items[id]))
	}
	s.mu.RUnlock()
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })

	out := make([]ExportedResource, 0)
	for _, item := range items {
		if matchesCollector(item, terms) {
			out = append(out, item)
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return CollectorResult{
		Selector: strings.TrimSpace(selector),
		Terms:    terms,
		Count:    len(out),
		Items:    out,
	}, nil
}

func parseCollectorSelector(selector string) ([]CollectorTerm, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, nil
	}
	normalized := strings.ReplaceAll(selector, ",", " and ")
	parts := strings.Split(normalized, " and ")
	out := make([]CollectorTerm, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pair := strings.SplitN(part, "=", 2)
		if len(pair) != 2 {
			return nil, errors.New("collector selector terms must use key=value syntax")
		}
		key := strings.ToLower(strings.TrimSpace(pair[0]))
		value := strings.ToLower(strings.Trim(strings.TrimSpace(pair[1]), `"'`))
		if key == "" || value == "" {
			return nil, errors.New("collector selector key and value are required")
		}
		out = append(out, CollectorTerm{Key: key, Value: value})
	}
	return out, nil
}

func matchesCollector(item ExportedResource, terms []CollectorTerm) bool {
	for _, term := range terms {
		key := strings.ToLower(strings.TrimSpace(term.Key))
		val := strings.ToLower(strings.TrimSpace(term.Value))
		if key == "" || val == "" {
			return false
		}
		switch {
		case key == "type":
			if item.Type != val {
				return false
			}
		case key == "host":
			if item.Host != val {
				return false
			}
		case key == "resource_id":
			if item.ResourceID != val {
				return false
			}
		case key == "source":
			if item.Source != val {
				return false
			}
		case strings.HasPrefix(key, "attrs."):
			attr := strings.TrimPrefix(key, "attrs.")
			if strings.TrimSpace(item.Attributes[attr]) != val {
				return false
			}
		case strings.HasPrefix(key, "attr."):
			attr := strings.TrimPrefix(key, "attr.")
			if strings.TrimSpace(item.Attributes[attr]) != val {
				return false
			}
		default:
			if strings.TrimSpace(item.Attributes[key]) != val {
				return false
			}
		}
	}
	return true
}

func (s *ExportedResourceStore) trimLocked() {
	if s.max <= 0 || len(s.ordered) <= s.max {
		return
	}
	drop := len(s.ordered) - s.max
	for i := 0; i < drop; i++ {
		id := s.ordered[0]
		s.ordered = s.ordered[1:]
		delete(s.items, id)
	}
}

func cloneExportedResource(in ExportedResource) ExportedResource {
	out := in
	out.Attributes = normalizeStringMap(in.Attributes)
	return out
}
