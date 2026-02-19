package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type AgentCompiledCatalog struct {
	ID          string    `json:"id"`
	ConfigPath  string    `json:"config_path"`
	PolicyGroup string    `json:"policy_group,omitempty"`
	AgentIDs    []string  `json:"agent_ids,omitempty"`
	ConfigSHA   string    `json:"config_sha"`
	Signature   string    `json:"signature,omitempty"`
	Signed      bool      `json:"signed"`
	CreatedAt   time.Time `json:"created_at"`
}

type AgentCatalogCompileInput struct {
	ConfigPath     string   `json:"config_path"`
	PolicyGroup    string   `json:"policy_group,omitempty"`
	AgentIDs       []string `json:"agent_ids,omitempty"`
	PrivateKeyPath string   `json:"private_key_path,omitempty"`
}

type AgentCatalogReplayInput struct {
	CatalogID     string `json:"catalog_id"`
	AgentID       string `json:"agent_id"`
	PublicKeyPath string `json:"public_key_path,omitempty"`
	AllowUnsigned bool   `json:"allow_unsigned,omitempty"`
	Disconnected  bool   `json:"disconnected,omitempty"`
}

type AgentCatalogReplayResult struct {
	Allowed  bool   `json:"allowed"`
	Verified bool   `json:"verified"`
	Reason   string `json:"reason,omitempty"`
}

type AgentCatalogReplayRecord struct {
	ID           string    `json:"id"`
	CatalogID    string    `json:"catalog_id"`
	AgentID      string    `json:"agent_id"`
	Allowed      bool      `json:"allowed"`
	Verified     bool      `json:"verified"`
	Reason       string    `json:"reason,omitempty"`
	Disconnected bool      `json:"disconnected"`
	CreatedAt    time.Time `json:"created_at"`
}

type AgentCatalogStore struct {
	mu         sync.RWMutex
	nextID     int64
	nextReplay int64
	catalogs   map[string]*AgentCompiledCatalog
	replays    []AgentCatalogReplayRecord
}

func NewAgentCatalogStore() *AgentCatalogStore {
	return &AgentCatalogStore{
		catalogs: map[string]*AgentCompiledCatalog{},
		replays:  make([]AgentCatalogReplayRecord, 0, 1024),
	}
}

func (s *AgentCatalogStore) CreateCatalog(item AgentCompiledCatalog) (AgentCompiledCatalog, error) {
	item.ConfigPath = strings.TrimSpace(item.ConfigPath)
	item.ConfigSHA = strings.TrimSpace(item.ConfigSHA)
	if item.ConfigPath == "" || item.ConfigSHA == "" {
		return AgentCompiledCatalog{}, errors.New("config_path and config_sha are required")
	}
	item.PolicyGroup = strings.TrimSpace(item.PolicyGroup)
	item.AgentIDs = normalizeStringSlice(item.AgentIDs)
	if item.Signed {
		item.Signature = strings.TrimSpace(item.Signature)
		if item.Signature == "" {
			return AgentCompiledCatalog{}, errors.New("signature is required when signed=true")
		}
	}
	item.CreatedAt = time.Now().UTC()

	s.mu.Lock()
	s.nextID++
	item.ID = "catalog-" + itoa(s.nextID)
	s.catalogs[item.ID] = &item
	s.mu.Unlock()
	return cloneCompiledCatalog(item), nil
}

func (s *AgentCatalogStore) ListCatalogs(limit int) []AgentCompiledCatalog {
	s.mu.RLock()
	out := make([]AgentCompiledCatalog, 0, len(s.catalogs))
	for _, item := range s.catalogs {
		out = append(out, cloneCompiledCatalog(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *AgentCatalogStore) GetCatalog(id string) (AgentCompiledCatalog, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.catalogs[strings.TrimSpace(id)]
	if !ok {
		return AgentCompiledCatalog{}, false
	}
	return cloneCompiledCatalog(*item), true
}

func (s *AgentCatalogStore) RecordReplay(in AgentCatalogReplayRecord) AgentCatalogReplayRecord {
	item := AgentCatalogReplayRecord{
		CatalogID:    strings.TrimSpace(in.CatalogID),
		AgentID:      strings.TrimSpace(in.AgentID),
		Allowed:      in.Allowed,
		Verified:     in.Verified,
		Reason:       strings.TrimSpace(in.Reason),
		Disconnected: in.Disconnected,
		CreatedAt:    time.Now().UTC(),
	}
	s.mu.Lock()
	s.nextReplay++
	item.ID = "catalog-replay-" + itoa(s.nextReplay)
	s.replays = append(s.replays, item)
	if len(s.replays) > 3000 {
		s.replays = s.replays[len(s.replays)-3000:]
	}
	s.mu.Unlock()
	return item
}

func (s *AgentCatalogStore) ListReplays(limit int) []AgentCatalogReplayRecord {
	s.mu.RLock()
	out := append([]AgentCatalogReplayRecord{}, s.replays...)
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func cloneCompiledCatalog(in AgentCompiledCatalog) AgentCompiledCatalog {
	out := in
	out.AgentIDs = append([]string{}, in.AgentIDs...)
	return out
}
