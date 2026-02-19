package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	PromotionStatusInProgress = "in_progress"
	PromotionStatusCompleted  = "completed"
)

type PromotionStageRecord struct {
	Stage     string    `json:"stage"`
	Actor     string    `json:"actor,omitempty"`
	Note      string    `json:"note,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type GitOpsPromotion struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Stages         []string               `json:"stages"`
	ArtifactDigest string                 `json:"artifact_digest"`
	CurrentStage   string                 `json:"current_stage"`
	CurrentIndex   int                    `json:"current_index"`
	Status         string                 `json:"status"`
	History        []PromotionStageRecord `json:"history"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

type GitOpsPromotionInput struct {
	Name           string   `json:"name"`
	Stages         []string `json:"stages,omitempty"`
	ArtifactDigest string   `json:"artifact_digest"`
	Actor          string   `json:"actor,omitempty"`
}

type GitOpsPromotionStore struct {
	mu         sync.RWMutex
	nextID     int64
	promotions map[string]*GitOpsPromotion
}

func NewGitOpsPromotionStore() *GitOpsPromotionStore {
	return &GitOpsPromotionStore{
		promotions: map[string]*GitOpsPromotion{},
	}
}

func (s *GitOpsPromotionStore) Create(in GitOpsPromotionInput) (GitOpsPromotion, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return GitOpsPromotion{}, errors.New("name is required")
	}
	digest := strings.ToLower(strings.TrimSpace(in.ArtifactDigest))
	if !isValidArtifactDigest(digest) {
		return GitOpsPromotion{}, errors.New("artifact_digest must be immutable sha256:<64-hex>")
	}
	stages, err := normalizePromotionStages(in.Stages)
	if err != nil {
		return GitOpsPromotion{}, err
	}

	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item := &GitOpsPromotion{
		ID:             "promotion-" + itoa(s.nextID),
		Name:           name,
		Stages:         stages,
		ArtifactDigest: digest,
		CurrentStage:   stages[0],
		CurrentIndex:   0,
		Status:         PromotionStatusInProgress,
		History: []PromotionStageRecord{
			{
				Stage:     stages[0],
				Actor:     strings.TrimSpace(in.Actor),
				Note:      "pipeline created with immutable artifact pin",
				Timestamp: now,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.promotions[item.ID] = item
	return clonePromotion(*item), nil
}

func (s *GitOpsPromotionStore) Get(id string) (GitOpsPromotion, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.promotions[strings.TrimSpace(id)]
	if !ok {
		return GitOpsPromotion{}, false
	}
	return clonePromotion(*item), true
}

func (s *GitOpsPromotionStore) List() []GitOpsPromotion {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]GitOpsPromotion, 0, len(s.promotions))
	for _, item := range s.promotions {
		out = append(out, clonePromotion(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *GitOpsPromotionStore) Advance(id, artifactDigest, actor, note string) (GitOpsPromotion, error) {
	id = strings.TrimSpace(id)
	artifactDigest = strings.ToLower(strings.TrimSpace(artifactDigest))

	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.promotions[id]
	if !ok {
		return GitOpsPromotion{}, errors.New("promotion pipeline not found")
	}
	if item.Status == PromotionStatusCompleted {
		return GitOpsPromotion{}, errors.New("promotion pipeline already completed")
	}
	if artifactDigest == "" {
		return GitOpsPromotion{}, errors.New("artifact_digest is required")
	}
	if artifactDigest != item.ArtifactDigest {
		return GitOpsPromotion{}, errors.New("artifact digest mismatch with immutable pipeline pin")
	}
	next := item.CurrentIndex + 1
	if next >= len(item.Stages) {
		item.Status = PromotionStatusCompleted
		item.UpdatedAt = time.Now().UTC()
		return clonePromotion(*item), nil
	}
	item.CurrentIndex = next
	item.CurrentStage = item.Stages[next]
	if next == len(item.Stages)-1 {
		item.Status = PromotionStatusCompleted
	}
	item.UpdatedAt = time.Now().UTC()
	item.History = append(item.History, PromotionStageRecord{
		Stage:     item.CurrentStage,
		Actor:     strings.TrimSpace(actor),
		Note:      strings.TrimSpace(note),
		Timestamp: item.UpdatedAt,
	})
	return clonePromotion(*item), nil
}

func normalizePromotionStages(in []string) ([]string, error) {
	if len(in) == 0 {
		in = []string{"staging", "canary", "production"}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, stage := range in {
		stage = strings.ToLower(strings.TrimSpace(stage))
		if stage == "" {
			continue
		}
		if _, ok := seen[stage]; ok {
			continue
		}
		seen[stage] = struct{}{}
		out = append(out, stage)
	}
	if len(out) < 2 {
		return nil, errors.New("at least two unique stages are required")
	}
	return out, nil
}

func clonePromotion(in GitOpsPromotion) GitOpsPromotion {
	out := in
	out.Stages = append([]string{}, in.Stages...)
	out.History = append([]PromotionStageRecord{}, in.History...)
	return out
}
