package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type GitOpsEnvironment struct {
	Name             string    `json:"name"`
	Branch           string    `json:"branch"`
	SourceConfigPath string    `json:"source_config_path"`
	OutputPath       string    `json:"output_path"`
	ContentSHA256    string    `json:"content_sha256"`
	LastJobID        string    `json:"last_job_id,omitempty"`
	MaterializedAt   time.Time `json:"materialized_at"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type GitOpsEnvironmentUpsert struct {
	Name             string `json:"name"`
	Branch           string `json:"branch"`
	SourceConfigPath string `json:"source_config_path"`
	OutputPath       string `json:"output_path"`
	ContentSHA256    string `json:"content_sha256"`
	LastJobID        string `json:"last_job_id,omitempty"`
}

type GitOpsEnvironmentStore struct {
	mu    sync.RWMutex
	items map[string]*GitOpsEnvironment
}

func NewGitOpsEnvironmentStore() *GitOpsEnvironmentStore {
	return &GitOpsEnvironmentStore{
		items: map[string]*GitOpsEnvironment{},
	}
}

func (s *GitOpsEnvironmentStore) Upsert(in GitOpsEnvironmentUpsert) (GitOpsEnvironment, bool, error) {
	name := strings.ToLower(strings.TrimSpace(in.Name))
	if name == "" {
		return GitOpsEnvironment{}, false, errors.New("environment name is required")
	}
	branch := strings.TrimSpace(in.Branch)
	if branch == "" {
		return GitOpsEnvironment{}, false, errors.New("branch is required")
	}
	source := strings.TrimSpace(in.SourceConfigPath)
	if source == "" {
		return GitOpsEnvironment{}, false, errors.New("source_config_path is required")
	}
	output := strings.TrimSpace(in.OutputPath)
	if output == "" {
		return GitOpsEnvironment{}, false, errors.New("output_path is required")
	}
	sha := strings.ToLower(strings.TrimSpace(in.ContentSHA256))
	if sha == "" {
		return GitOpsEnvironment{}, false, errors.New("content_sha256 is required")
	}

	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	item, exists := s.items[name]
	if !exists {
		item = &GitOpsEnvironment{
			Name:      name,
			CreatedAt: now,
		}
		s.items[name] = item
	}
	item.Branch = branch
	item.SourceConfigPath = source
	item.OutputPath = output
	item.ContentSHA256 = sha
	item.MaterializedAt = now
	item.UpdatedAt = now
	item.LastJobID = strings.TrimSpace(in.LastJobID)
	return cloneGitOpsEnvironment(*item), !exists, nil
}

func (s *GitOpsEnvironmentStore) Get(name string) (GitOpsEnvironment, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[name]
	if !ok {
		return GitOpsEnvironment{}, false
	}
	return cloneGitOpsEnvironment(*item), true
}

func (s *GitOpsEnvironmentStore) List() []GitOpsEnvironment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]GitOpsEnvironment, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, cloneGitOpsEnvironment(*item))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func cloneGitOpsEnvironment(in GitOpsEnvironment) GitOpsEnvironment {
	return in
}
