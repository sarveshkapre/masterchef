package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ArtifactDeploymentInput struct {
	Environment  string   `json:"environment"`
	ArtifactRef  string   `json:"artifact_ref"`
	Checksum     string   `json:"checksum"`
	Targets      []string `json:"targets"`
	StageSize    int      `json:"stage_size,omitempty"`
	AllowPartial bool     `json:"allow_partial,omitempty"`
}

type ArtifactDeployment struct {
	ID           string    `json:"id"`
	Environment  string    `json:"environment"`
	ArtifactRef  string    `json:"artifact_ref"`
	Checksum     string    `json:"checksum"`
	Targets      []string  `json:"targets"`
	StageSize    int       `json:"stage_size"`
	AllowPartial bool      `json:"allow_partial"`
	CreatedAt    time.Time `json:"created_at"`
}

type ArtifactDeploymentPlan struct {
	Allowed        bool                      `json:"allowed"`
	DeploymentID   string                    `json:"deployment_id,omitempty"`
	Environment    string                    `json:"environment"`
	ArtifactRef    string                    `json:"artifact_ref"`
	ChecksumPinned bool                      `json:"checksum_pinned"`
	BlockedReason  string                    `json:"blocked_reason,omitempty"`
	Stages         []ArtifactDeploymentStage `json:"stages,omitempty"`
}

type ArtifactDeploymentStage struct {
	Index   int      `json:"index"`
	Targets []string `json:"targets"`
	Reason  string   `json:"reason"`
}

type ArtifactDeploymentStore struct {
	mu          sync.RWMutex
	nextID      int64
	deployments map[string]*ArtifactDeployment
}

func NewArtifactDeploymentStore() *ArtifactDeploymentStore {
	return &ArtifactDeploymentStore{deployments: map[string]*ArtifactDeployment{}}
}

func (s *ArtifactDeploymentStore) Create(in ArtifactDeploymentInput) (ArtifactDeployment, ArtifactDeploymentPlan, error) {
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	artifactRef := strings.TrimSpace(in.ArtifactRef)
	checksum := strings.ToLower(strings.TrimSpace(in.Checksum))
	if environment == "" || artifactRef == "" {
		return ArtifactDeployment{}, ArtifactDeploymentPlan{}, errors.New("environment and artifact_ref are required")
	}
	targets := normalizeOrderedTargets(in.Targets)
	if len(targets) == 0 {
		return ArtifactDeployment{}, ArtifactDeploymentPlan{}, errors.New("targets are required")
	}
	stageSize := in.StageSize
	if stageSize <= 0 {
		stageSize = 1
	}
	if stageSize > len(targets) {
		stageSize = len(targets)
	}
	item := ArtifactDeployment{
		Environment:  environment,
		ArtifactRef:  artifactRef,
		Checksum:     checksum,
		Targets:      targets,
		StageSize:    stageSize,
		AllowPartial: in.AllowPartial,
		CreatedAt:    time.Now().UTC(),
	}

	s.mu.Lock()
	s.nextID++
	item.ID = "artifact-deploy-" + itoa(s.nextID)
	s.deployments[item.ID] = &item
	s.mu.Unlock()
	return item, s.plan(item), nil
}

func (s *ArtifactDeploymentStore) List() []ArtifactDeployment {
	s.mu.RLock()
	out := make([]ArtifactDeployment, 0, len(s.deployments))
	for _, item := range s.deployments {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *ArtifactDeploymentStore) Get(id string) (ArtifactDeployment, bool) {
	s.mu.RLock()
	item, ok := s.deployments[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return ArtifactDeployment{}, false
	}
	return *item, true
}

func (s *ArtifactDeploymentStore) PlanByID(id string) (ArtifactDeploymentPlan, error) {
	item, ok := s.Get(id)
	if !ok {
		return ArtifactDeploymentPlan{}, errors.New("artifact deployment not found")
	}
	return s.plan(item), nil
}

func (s *ArtifactDeploymentStore) plan(item ArtifactDeployment) ArtifactDeploymentPlan {
	plan := ArtifactDeploymentPlan{
		Allowed:        true,
		DeploymentID:   item.ID,
		Environment:    item.Environment,
		ArtifactRef:    item.ArtifactRef,
		ChecksumPinned: item.Checksum != "",
	}
	if item.Checksum == "" {
		plan.Allowed = false
		plan.BlockedReason = "checksum pin is required for artifact deployment"
		return plan
	}
	stages := make([]ArtifactDeploymentStage, 0, len(item.Targets))
	stage := 1
	for i := 0; i < len(item.Targets); i += item.StageSize {
		end := i + item.StageSize
		if end > len(item.Targets) {
			end = len(item.Targets)
		}
		stages = append(stages, ArtifactDeploymentStage{
			Index:   stage,
			Targets: append([]string{}, item.Targets[i:end]...),
			Reason:  "checksum-verified staged artifact rollout",
		})
		stage++
	}
	plan.Stages = stages
	return plan
}

func normalizeOrderedTargets(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, raw := range in {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
