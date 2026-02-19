package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type OfflineModeConfig struct {
	Enabled    bool      `json:"enabled"`
	AirGapped  bool      `json:"air_gapped"`
	MirrorPath string    `json:"mirror_path,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type OfflineBundle struct {
	ID          string    `json:"id"`
	ManifestSHA string    `json:"manifest_sha"`
	Items       []string  `json:"items"`
	Artifacts   []string  `json:"artifacts,omitempty"`
	Signed      bool      `json:"signed"`
	Signature   string    `json:"signature,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type OfflineBundleInput struct {
	ManifestSHA string   `json:"manifest_sha"`
	Items       []string `json:"items"`
	Artifacts   []string `json:"artifacts,omitempty"`
	Signed      bool     `json:"signed"`
	Signature   string   `json:"signature,omitempty"`
}

type OfflineBundleVerifyResult struct {
	BundleID string `json:"bundle_id"`
	Verified bool   `json:"verified"`
	Reason   string `json:"reason,omitempty"`
}

type OfflineStore struct {
	mu      sync.RWMutex
	nextID  int64
	mode    OfflineModeConfig
	bundles map[string]*OfflineBundle
}

func NewOfflineStore() *OfflineStore {
	return &OfflineStore{
		mode:    OfflineModeConfig{Enabled: false, AirGapped: false, UpdatedAt: time.Now().UTC()},
		bundles: map[string]*OfflineBundle{},
	}
}

func (s *OfflineStore) Mode() OfflineModeConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

func (s *OfflineStore) SetMode(in OfflineModeConfig) OfflineModeConfig {
	in.MirrorPath = strings.TrimSpace(in.MirrorPath)
	in.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	s.mode = in
	s.mu.Unlock()
	return in
}

func (s *OfflineStore) CreateBundle(in OfflineBundleInput) (OfflineBundle, error) {
	sha := strings.TrimSpace(in.ManifestSHA)
	if sha == "" {
		return OfflineBundle{}, errors.New("manifest_sha is required")
	}
	items := normalizeStringSlice(in.Items)
	if len(items) == 0 {
		return OfflineBundle{}, errors.New("items are required")
	}
	artifacts := normalizeStringSlice(in.Artifacts)
	sig := strings.TrimSpace(in.Signature)
	if in.Signed && sig == "" {
		return OfflineBundle{}, errors.New("signature is required for signed bundles")
	}
	item := OfflineBundle{
		ManifestSHA: sha,
		Items:       items,
		Artifacts:   artifacts,
		Signed:      in.Signed,
		Signature:   sig,
		CreatedAt:   time.Now().UTC(),
	}
	s.mu.Lock()
	s.nextID++
	item.ID = "offline-bundle-" + itoa(s.nextID)
	s.bundles[item.ID] = &item
	s.mu.Unlock()
	return cloneOfflineBundle(item), nil
}

func (s *OfflineStore) ListBundles(limit int) []OfflineBundle {
	s.mu.RLock()
	out := make([]OfflineBundle, 0, len(s.bundles))
	for _, item := range s.bundles {
		out = append(out, cloneOfflineBundle(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *OfflineStore) GetBundle(id string) (OfflineBundle, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.bundles[strings.TrimSpace(id)]
	if !ok {
		return OfflineBundle{}, false
	}
	return cloneOfflineBundle(*item), true
}

func cloneOfflineBundle(in OfflineBundle) OfflineBundle {
	out := in
	out.Items = append([]string{}, in.Items...)
	out.Artifacts = append([]string{}, in.Artifacts...)
	return out
}
