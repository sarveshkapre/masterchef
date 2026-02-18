package control

import (
	"errors"
	"strings"
	"sync"
	"time"
)

type BulkOperation struct {
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id"`
	Params     map[string]any `json:"params,omitempty"`
}

type BulkOperationPreview struct {
	Operation BulkOperation `json:"operation"`
	Ready     bool          `json:"ready"`
	Reason    string        `json:"reason,omitempty"`
}

type BulkPreview struct {
	Token      string                 `json:"token"`
	Name       string                 `json:"name"`
	CreatedAt  time.Time              `json:"created_at"`
	ExpiresAt  time.Time              `json:"expires_at"`
	Ready      bool                   `json:"ready"`
	Operations []BulkOperationPreview `json:"operations"`
	Conflicts  []string               `json:"conflicts,omitempty"`
}

type BulkExecutionResult struct {
	Operation BulkOperation `json:"operation"`
	Applied   bool          `json:"applied"`
	Error     string        `json:"error,omitempty"`
}

type BulkManager struct {
	mu         sync.RWMutex
	nextID     int64
	previews   map[string]BulkPreview
	defaultTTL time.Duration
}

func NewBulkManager(defaultTTL time.Duration) *BulkManager {
	if defaultTTL <= 0 {
		defaultTTL = 15 * time.Minute
	}
	return &BulkManager{
		previews:   map[string]BulkPreview{},
		defaultTTL: defaultTTL,
	}
}

func (m *BulkManager) SavePreview(name string, operations []BulkOperationPreview, conflicts []string) BulkPreview {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	now := time.Now().UTC()
	token := "bulk-preview-" + itoa(m.nextID)
	ready := true
	for _, op := range operations {
		if !op.Ready {
			ready = false
			break
		}
	}
	if len(conflicts) > 0 {
		ready = false
	}
	item := BulkPreview{
		Token:      token,
		Name:       strings.TrimSpace(name),
		CreatedAt:  now,
		ExpiresAt:  now.Add(m.defaultTTL),
		Ready:      ready,
		Operations: append([]BulkOperationPreview{}, operations...),
		Conflicts:  append([]string{}, conflicts...),
	}
	m.previews[token] = item
	return item
}

func (m *BulkManager) GetPreview(token string) (BulkPreview, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.previews[strings.TrimSpace(token)]
	if !ok {
		return BulkPreview{}, errors.New("preview token not found")
	}
	if time.Now().UTC().After(item.ExpiresAt) {
		delete(m.previews, item.Token)
		return BulkPreview{}, errors.New("preview token expired")
	}
	return item, nil
}

func (m *BulkManager) ConsumePreview(token string) (BulkPreview, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	token = strings.TrimSpace(token)
	item, ok := m.previews[token]
	if !ok {
		return BulkPreview{}, errors.New("preview token not found")
	}
	if time.Now().UTC().After(item.ExpiresAt) {
		delete(m.previews, token)
		return BulkPreview{}, errors.New("preview token expired")
	}
	delete(m.previews, token)
	return item, nil
}
