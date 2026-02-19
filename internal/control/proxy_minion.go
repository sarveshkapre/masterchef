package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ProxyMinionBindingInput struct {
	ProxyID   string            `json:"proxy_id"`
	DeviceID  string            `json:"device_id"`
	Transport string            `json:"transport"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Status    string            `json:"status,omitempty"`
}

type ProxyMinionBinding struct {
	ID         string            `json:"id"`
	ProxyID    string            `json:"proxy_id"`
	DeviceID   string            `json:"device_id"`
	Transport  string            `json:"transport"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Status     string            `json:"status"`
	UpdatedAt  time.Time         `json:"updated_at"`
	Registered time.Time         `json:"registered"`
}

type ProxyMinionDispatchRequest struct {
	DeviceID   string `json:"device_id"`
	ConfigPath string `json:"config_path"`
	Priority   string `json:"priority,omitempty"`
	Force      bool   `json:"force,omitempty"`
}

type ProxyMinionDispatchRecord struct {
	ID         string    `json:"id"`
	BindingID  string    `json:"binding_id"`
	ProxyID    string    `json:"proxy_id"`
	DeviceID   string    `json:"device_id"`
	ConfigPath string    `json:"config_path"`
	Status     string    `json:"status"`
	JobID      string    `json:"job_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type ProxyMinionStore struct {
	mu         sync.RWMutex
	nextID     int64
	nextRecord int64
	bindings   map[string]*ProxyMinionBinding
	byDevice   map[string]string
	records    []ProxyMinionDispatchRecord
}

func NewProxyMinionStore() *ProxyMinionStore {
	return &ProxyMinionStore{
		bindings: map[string]*ProxyMinionBinding{},
		byDevice: map[string]string{},
		records:  make([]ProxyMinionDispatchRecord, 0, 1024),
	}
}

func (s *ProxyMinionStore) UpsertBinding(in ProxyMinionBindingInput) (ProxyMinionBinding, error) {
	proxyID := strings.TrimSpace(in.ProxyID)
	deviceID := strings.TrimSpace(in.DeviceID)
	transport := strings.ToLower(strings.TrimSpace(in.Transport))
	if proxyID == "" || deviceID == "" || transport == "" {
		return ProxyMinionBinding{}, errors.New("proxy_id, device_id, and transport are required")
	}
	status := strings.ToLower(strings.TrimSpace(in.Status))
	if status == "" {
		status = "active"
	}
	meta := map[string]string{}
	for k, v := range in.Metadata {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		meta[key] = strings.TrimSpace(v)
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	if existingID, ok := s.byDevice[deviceID]; ok {
		item := s.bindings[existingID]
		item.ProxyID = proxyID
		item.Transport = transport
		item.Status = status
		item.Metadata = meta
		item.UpdatedAt = now
		return cloneProxyBinding(*item), nil
	}
	s.nextID++
	item := ProxyMinionBinding{
		ID:         "proxy-binding-" + itoa(s.nextID),
		ProxyID:    proxyID,
		DeviceID:   deviceID,
		Transport:  transport,
		Metadata:   meta,
		Status:     status,
		UpdatedAt:  now,
		Registered: now,
	}
	s.bindings[item.ID] = &item
	s.byDevice[deviceID] = item.ID
	return cloneProxyBinding(item), nil
}

func (s *ProxyMinionStore) ListBindings() []ProxyMinionBinding {
	s.mu.RLock()
	out := make([]ProxyMinionBinding, 0, len(s.bindings))
	for _, item := range s.bindings {
		out = append(out, cloneProxyBinding(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (s *ProxyMinionStore) GetBinding(id string) (ProxyMinionBinding, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.bindings[strings.TrimSpace(id)]
	if !ok {
		return ProxyMinionBinding{}, false
	}
	return cloneProxyBinding(*item), true
}

func (s *ProxyMinionStore) ResolveDevice(deviceID string) (ProxyMinionBinding, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byDevice[strings.TrimSpace(deviceID)]
	if !ok {
		return ProxyMinionBinding{}, false
	}
	item := s.bindings[id]
	if item == nil {
		return ProxyMinionBinding{}, false
	}
	return cloneProxyBinding(*item), true
}

func (s *ProxyMinionStore) RecordDispatch(binding ProxyMinionBinding, req ProxyMinionDispatchRequest, status, jobID string) ProxyMinionDispatchRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextRecord++
	item := ProxyMinionDispatchRecord{
		ID:         "proxy-dispatch-" + itoa(s.nextRecord),
		BindingID:  binding.ID,
		ProxyID:    binding.ProxyID,
		DeviceID:   binding.DeviceID,
		ConfigPath: strings.TrimSpace(req.ConfigPath),
		Status:     strings.TrimSpace(status),
		JobID:      strings.TrimSpace(jobID),
		CreatedAt:  time.Now().UTC(),
	}
	s.records = append(s.records, item)
	if len(s.records) > 3000 {
		s.records = s.records[len(s.records)-3000:]
	}
	return item
}

func (s *ProxyMinionStore) ListDispatches(limit int) []ProxyMinionDispatchRecord {
	s.mu.RLock()
	out := append([]ProxyMinionDispatchRecord{}, s.records...)
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func cloneProxyBinding(in ProxyMinionBinding) ProxyMinionBinding {
	out := in
	out.Metadata = map[string]string{}
	for k, v := range in.Metadata {
		out.Metadata[k] = v
	}
	return out
}
