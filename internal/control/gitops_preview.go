package control

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	PreviewStatusActive   = "active"
	PreviewStatusExpired  = "expired"
	PreviewStatusPromoted = "promoted"
	PreviewStatusClosed   = "closed"
)

type GitOpsPreview struct {
	ID             string    `json:"id"`
	Branch         string    `json:"branch"`
	Environment    string    `json:"environment"`
	ConfigPath     string    `json:"config_path,omitempty"`
	ArtifactDigest string    `json:"artifact_digest,omitempty"`
	LastJobID      string    `json:"last_job_id,omitempty"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type GitOpsPreviewInput struct {
	Branch         string `json:"branch"`
	Environment    string `json:"environment,omitempty"`
	ConfigPath     string `json:"config_path,omitempty"`
	ArtifactDigest string `json:"artifact_digest,omitempty"`
	TTLSeconds     int    `json:"ttl_seconds,omitempty"`
}

type GitOpsPreviewStore struct {
	mu       sync.RWMutex
	nextID   int64
	previews map[string]*GitOpsPreview
}

func NewGitOpsPreviewStore() *GitOpsPreviewStore {
	return &GitOpsPreviewStore{
		previews: map[string]*GitOpsPreview{},
	}
}

func (s *GitOpsPreviewStore) Create(in GitOpsPreviewInput) (GitOpsPreview, error) {
	branch := strings.TrimSpace(in.Branch)
	if branch == "" {
		return GitOpsPreview{}, errors.New("branch is required")
	}
	env := strings.TrimSpace(in.Environment)
	if env == "" {
		env = "preview"
	}
	ttl := in.TTLSeconds
	if ttl <= 0 {
		ttl = 24 * 3600
	}
	if ttl > 14*24*3600 {
		ttl = 14 * 24 * 3600
	}
	digest := strings.TrimSpace(strings.ToLower(in.ArtifactDigest))
	if digest != "" && !isValidArtifactDigest(digest) {
		return GitOpsPreview{}, errors.New("artifact_digest must be immutable sha256:<64-hex>")
	}

	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item := &GitOpsPreview{
		ID:             "preview-" + itoa(s.nextID),
		Branch:         branch,
		Environment:    env,
		ConfigPath:     strings.TrimSpace(in.ConfigPath),
		ArtifactDigest: digest,
		Status:         PreviewStatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
		ExpiresAt:      now.Add(time.Duration(ttl) * time.Second),
	}
	s.previews[item.ID] = item
	return clonePreview(*item), nil
}

func (s *GitOpsPreviewStore) Get(id string) (GitOpsPreview, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.previews[strings.TrimSpace(id)]
	if !ok {
		return GitOpsPreview{}, false
	}
	return clonePreview(*item), true
}

func (s *GitOpsPreviewStore) List(includeExpired bool) []GitOpsPreview {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]GitOpsPreview, 0, len(s.previews))
	for _, item := range s.previews {
		if item.Status == PreviewStatusActive && now.After(item.ExpiresAt) {
			item.Status = PreviewStatusExpired
			item.UpdatedAt = now
		}
		if !includeExpired && item.Status == PreviewStatusExpired {
			continue
		}
		out = append(out, clonePreview(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *GitOpsPreviewStore) SetStatus(id, status string) (GitOpsPreview, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case PreviewStatusActive, PreviewStatusExpired, PreviewStatusPromoted, PreviewStatusClosed:
	default:
		return GitOpsPreview{}, errors.New("unsupported preview status")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.previews[strings.TrimSpace(id)]
	if !ok {
		return GitOpsPreview{}, errors.New("preview not found")
	}
	item.Status = status
	item.UpdatedAt = time.Now().UTC()
	return clonePreview(*item), nil
}

func (s *GitOpsPreviewStore) AttachJob(id, jobID string) (GitOpsPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.previews[strings.TrimSpace(id)]
	if !ok {
		return GitOpsPreview{}, errors.New("preview not found")
	}
	item.LastJobID = strings.TrimSpace(jobID)
	item.UpdatedAt = time.Now().UTC()
	return clonePreview(*item), nil
}

func clonePreview(in GitOpsPreview) GitOpsPreview {
	return in
}

var sha256DigestRe = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func isValidArtifactDigest(digest string) bool {
	return sha256DigestRe.MatchString(strings.TrimSpace(strings.ToLower(digest)))
}
