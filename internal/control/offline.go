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

type OfflineMirrorInput struct {
	Name            string   `json:"name"`
	Upstream        string   `json:"upstream"`
	MirrorPath      string   `json:"mirror_path"`
	IncludePatterns []string `json:"include_patterns,omitempty"`
}

type OfflineMirror struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Upstream        string    `json:"upstream"`
	MirrorPath      string    `json:"mirror_path"`
	IncludePatterns []string  `json:"include_patterns,omitempty"`
	LastSyncAt      time.Time `json:"last_sync_at,omitempty"`
	LastSyncStatus  string    `json:"last_sync_status,omitempty"`
	SyncedArtifacts int       `json:"synced_artifacts,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type OfflineMirrorSyncInput struct {
	MirrorID  string   `json:"mirror_id"`
	Artifacts []string `json:"artifacts,omitempty"`
	DryRun    bool     `json:"dry_run,omitempty"`
}

type OfflineMirrorSyncResult struct {
	MirrorID         string    `json:"mirror_id"`
	ArtifactCount    int       `json:"artifact_count"`
	SyncedArtifacts  int       `json:"synced_artifacts"`
	MissingArtifacts []string  `json:"missing_artifacts,omitempty"`
	Status           string    `json:"status"`
	StartedAt        time.Time `json:"started_at"`
	CompletedAt      time.Time `json:"completed_at"`
}

type OfflineStore struct {
	mu         sync.RWMutex
	nextID     int64
	nextMirror int64
	mode       OfflineModeConfig
	bundles    map[string]*OfflineBundle
	mirrors    map[string]*OfflineMirror
}

func NewOfflineStore() *OfflineStore {
	return &OfflineStore{
		mode:    OfflineModeConfig{Enabled: false, AirGapped: false, UpdatedAt: time.Now().UTC()},
		bundles: map[string]*OfflineBundle{},
		mirrors: map[string]*OfflineMirror{},
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

func (s *OfflineStore) UpsertMirror(in OfflineMirrorInput) (OfflineMirror, error) {
	name := strings.TrimSpace(in.Name)
	upstream := strings.TrimSpace(in.Upstream)
	mirrorPath := strings.TrimSpace(in.MirrorPath)
	if name == "" || upstream == "" || mirrorPath == "" {
		return OfflineMirror{}, errors.New("name, upstream, and mirror_path are required")
	}
	item := OfflineMirror{
		Name:            name,
		Upstream:        upstream,
		MirrorPath:      mirrorPath,
		IncludePatterns: normalizeStringSlice(in.IncludePatterns),
		UpdatedAt:       time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, existing := range s.mirrors {
		if strings.EqualFold(existing.Name, name) {
			item.ID = id
			item.LastSyncAt = existing.LastSyncAt
			item.LastSyncStatus = existing.LastSyncStatus
			item.SyncedArtifacts = existing.SyncedArtifacts
			s.mirrors[id] = &item
			return cloneOfflineMirror(item), nil
		}
	}
	s.nextMirror++
	item.ID = "offline-mirror-" + itoa(s.nextMirror)
	s.mirrors[item.ID] = &item
	return cloneOfflineMirror(item), nil
}

func (s *OfflineStore) ListMirrors() []OfflineMirror {
	s.mu.RLock()
	out := make([]OfflineMirror, 0, len(s.mirrors))
	for _, item := range s.mirrors {
		out = append(out, cloneOfflineMirror(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (s *OfflineStore) GetMirror(id string) (OfflineMirror, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.mirrors[strings.TrimSpace(id)]
	if !ok {
		return OfflineMirror{}, false
	}
	return cloneOfflineMirror(*item), true
}

func (s *OfflineStore) SyncMirror(in OfflineMirrorSyncInput) (OfflineMirrorSyncResult, error) {
	mirrorID := strings.TrimSpace(in.MirrorID)
	if mirrorID == "" {
		return OfflineMirrorSyncResult{}, errors.New("mirror_id is required")
	}
	started := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	mirror, ok := s.mirrors[mirrorID]
	if !ok {
		return OfflineMirrorSyncResult{}, errors.New("offline mirror not found")
	}
	artifacts := normalizeStringSlice(in.Artifacts)
	if len(artifacts) == 0 {
		artifacts = collectOfflineArtifactsLocked(s.bundles)
	}
	missing := make([]string, 0)
	validCount := 0
	for _, artifact := range artifacts {
		if strings.Contains(artifact, "@sha256:") {
			validCount++
			continue
		}
		missing = append(missing, artifact)
	}
	status := "succeeded"
	synced := validCount
	if in.DryRun {
		status = "planned"
		synced = 0
	} else if len(missing) > 0 {
		status = "partial"
	}
	completed := time.Now().UTC()
	mirror.LastSyncAt = completed
	mirror.LastSyncStatus = status
	mirror.SyncedArtifacts = synced
	mirror.UpdatedAt = completed

	return OfflineMirrorSyncResult{
		MirrorID:         mirror.ID,
		ArtifactCount:    len(artifacts),
		SyncedArtifacts:  synced,
		MissingArtifacts: missing,
		Status:           status,
		StartedAt:        started,
		CompletedAt:      completed,
	}, nil
}

func collectOfflineArtifactsLocked(bundles map[string]*OfflineBundle) []string {
	out := make([]string, 0, len(bundles))
	seen := map[string]struct{}{}
	for _, bundle := range bundles {
		for _, artifact := range bundle.Artifacts {
			if _, ok := seen[artifact]; ok {
				continue
			}
			seen[artifact] = struct{}{}
			out = append(out, artifact)
		}
	}
	sort.Strings(out)
	return out
}

func cloneOfflineMirror(in OfflineMirror) OfflineMirror {
	out := in
	out.IncludePatterns = append([]string{}, in.IncludePatterns...)
	return out
}
