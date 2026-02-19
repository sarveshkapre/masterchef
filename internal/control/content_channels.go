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

type ContentChannel struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Curated     bool      `json:"curated"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ChannelSyncPolicy struct {
	Channel   string    `json:"channel"`
	Allowlist []string  `json:"allowlist,omitempty"`
	Blocklist []string  `json:"blocklist,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OrgSyncRemote struct {
	ID              string    `json:"id"`
	Organization    string    `json:"organization"`
	Channel         string    `json:"channel"`
	Name            string    `json:"name"`
	URL             string    `json:"url"`
	TokenConfigured bool      `json:"token_configured"`
	Enabled         bool      `json:"enabled"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type OrgSyncRemoteInput struct {
	Organization string `json:"organization"`
	Channel      string `json:"channel"`
	Name         string `json:"name"`
	URL          string `json:"url"`
	APIToken     string `json:"api_token"`
	Enabled      bool   `json:"enabled"`
}

type contentRemoteRecord struct {
	OrgSyncRemote
	TokenHash string
}

type ContentChannelStore struct {
	mu       sync.RWMutex
	nextID   int64
	channels map[string]ContentChannel
	policies map[string]ChannelSyncPolicy
	remotes  map[string]*contentRemoteRecord
}

func NewContentChannelStore() *ContentChannelStore {
	now := time.Now().UTC()
	channels := map[string]ContentChannel{
		"certified": {
			Name:        "certified",
			Description: "Rigorously certified content channel for production-critical modules/providers.",
			Curated:     true,
			UpdatedAt:   now,
		},
		"validated": {
			Name:        "validated",
			Description: "Validated content channel for pre-production and staged environments.",
			Curated:     true,
			UpdatedAt:   now,
		},
		"community": {
			Name:        "community",
			Description: "Community content channel with policy-controlled synchronization.",
			Curated:     false,
			UpdatedAt:   now,
		},
	}
	policies := map[string]ChannelSyncPolicy{
		"certified": {Channel: "certified", Allowlist: []string{}, Blocklist: []string{}, UpdatedAt: now},
		"validated": {Channel: "validated", Allowlist: []string{}, Blocklist: []string{}, UpdatedAt: now},
		"community": {Channel: "community", Allowlist: []string{}, Blocklist: []string{}, UpdatedAt: now},
	}
	return &ContentChannelStore{
		channels: channels,
		policies: policies,
		remotes:  map[string]*contentRemoteRecord{},
	}
}

func (s *ContentChannelStore) ListChannels() []ContentChannel {
	s.mu.RLock()
	out := make([]ContentChannel, 0, len(s.channels))
	for _, item := range s.channels {
		out = append(out, item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *ContentChannelStore) GetPolicy(channel string) (ChannelSyncPolicy, error) {
	channel = normalizeChannelName(channel)
	if channel == "" {
		return ChannelSyncPolicy{}, errors.New("channel is required")
	}
	s.mu.RLock()
	policy, ok := s.policies[channel]
	s.mu.RUnlock()
	if !ok {
		return ChannelSyncPolicy{}, errors.New("channel not found")
	}
	policy.Allowlist = append([]string{}, policy.Allowlist...)
	policy.Blocklist = append([]string{}, policy.Blocklist...)
	return policy, nil
}

func (s *ContentChannelStore) SetPolicy(in ChannelSyncPolicy) (ChannelSyncPolicy, error) {
	channel := normalizeChannelName(in.Channel)
	if channel == "" {
		return ChannelSyncPolicy{}, errors.New("channel is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channels[channel]; !ok {
		return ChannelSyncPolicy{}, errors.New("channel not found")
	}
	item := ChannelSyncPolicy{
		Channel:   channel,
		Allowlist: normalizeStringSlice(in.Allowlist),
		Blocklist: normalizeStringSlice(in.Blocklist),
		UpdatedAt: time.Now().UTC(),
	}
	s.policies[channel] = item
	item.Allowlist = append([]string{}, item.Allowlist...)
	item.Blocklist = append([]string{}, item.Blocklist...)
	return item, nil
}

func (s *ContentChannelStore) UpsertRemote(in OrgSyncRemoteInput) (OrgSyncRemote, error) {
	org := strings.ToLower(strings.TrimSpace(in.Organization))
	channel := normalizeChannelName(in.Channel)
	name := strings.TrimSpace(in.Name)
	url := strings.TrimSpace(in.URL)
	if org == "" || channel == "" || name == "" || url == "" {
		return OrgSyncRemote{}, errors.New("organization, channel, name, and url are required")
	}
	if !strings.HasPrefix(strings.ToLower(url), "https://") {
		return OrgSyncRemote{}, errors.New("url must be https")
	}
	if in.APIToken == "" {
		return OrgSyncRemote{}, errors.New("api_token is required")
	}
	tokenHash := hashToken(in.APIToken)

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channels[channel]; !ok {
		return OrgSyncRemote{}, errors.New("channel not found")
	}
	now := time.Now().UTC()
	// Upsert by (organization, channel, name).
	for _, existing := range s.remotes {
		if existing.Organization == org && existing.Channel == channel && existing.Name == name {
			existing.URL = url
			existing.TokenHash = tokenHash
			existing.TokenConfigured = true
			existing.Enabled = in.Enabled
			existing.UpdatedAt = now
			return existing.OrgSyncRemote, nil
		}
	}
	s.nextID++
	item := OrgSyncRemote{
		ID:              "sync-remote-" + itoa(s.nextID),
		Organization:    org,
		Channel:         channel,
		Name:            name,
		URL:             url,
		TokenConfigured: true,
		Enabled:         in.Enabled,
		UpdatedAt:       now,
	}
	s.remotes[item.ID] = &contentRemoteRecord{
		OrgSyncRemote: item,
		TokenHash:     tokenHash,
	}
	return item, nil
}

func (s *ContentChannelStore) ListRemotes(organization, channel string) []OrgSyncRemote {
	org := strings.ToLower(strings.TrimSpace(organization))
	channel = normalizeChannelName(channel)
	s.mu.RLock()
	out := make([]OrgSyncRemote, 0, len(s.remotes))
	for _, item := range s.remotes {
		if org != "" && item.Organization != org {
			continue
		}
		if channel != "" && item.Channel != channel {
			continue
		}
		out = append(out, item.OrgSyncRemote)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Organization != out[j].Organization {
			return out[i].Organization < out[j].Organization
		}
		if out[i].Channel != out[j].Channel {
			return out[i].Channel < out[j].Channel
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (s *ContentChannelStore) GetRemote(id string) (OrgSyncRemote, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return OrgSyncRemote{}, errors.New("remote id is required")
	}
	s.mu.RLock()
	item, ok := s.remotes[id]
	s.mu.RUnlock()
	if !ok {
		return OrgSyncRemote{}, errors.New("sync remote not found")
	}
	return item.OrgSyncRemote, nil
}

func (s *ContentChannelStore) RotateRemoteToken(id, newToken string) (OrgSyncRemote, error) {
	id = strings.TrimSpace(id)
	newToken = strings.TrimSpace(newToken)
	if id == "" || newToken == "" {
		return OrgSyncRemote{}, errors.New("id and token are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.remotes[id]
	if !ok {
		return OrgSyncRemote{}, errors.New("sync remote not found")
	}
	item.TokenHash = hashToken(newToken)
	item.TokenConfigured = true
	item.UpdatedAt = time.Now().UTC()
	return item.OrgSyncRemote, nil
}

func normalizeChannelName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "certified":
		return "certified"
	case "validated":
		return "validated"
	case "community":
		return "community"
	default:
		return ""
	}
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
