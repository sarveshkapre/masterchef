package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type RolloutPolicyInput struct {
	Environment    string `json:"environment"`
	Strategy       string `json:"strategy"` // blue-green|canary|rolling
	Mode           string `json:"mode"`     // serial|batch|percentage
	BatchSize      int    `json:"batch_size,omitempty"`
	BatchPercent   int    `json:"batch_percent,omitempty"`
	CanaryPercent  int    `json:"canary_percent,omitempty"`
	MaxUnavailable int    `json:"max_unavailable,omitempty"`
}

type RolloutPolicy struct {
	ID             string    `json:"id"`
	Environment    string    `json:"environment"`
	Strategy       string    `json:"strategy"`
	Mode           string    `json:"mode"`
	BatchSize      int       `json:"batch_size"`
	BatchPercent   int       `json:"batch_percent"`
	CanaryPercent  int       `json:"canary_percent"`
	MaxUnavailable int       `json:"max_unavailable"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type RolloutPlanInput struct {
	Environment string   `json:"environment"`
	Targets     []string `json:"targets"`
}

type RolloutWave struct {
	Index   int      `json:"index"`
	Targets []string `json:"targets"`
	Phase   string   `json:"phase"`
	Reason  string   `json:"reason"`
}

type RolloutPlan struct {
	Allowed       bool          `json:"allowed"`
	Environment   string        `json:"environment"`
	PolicyID      string        `json:"policy_id,omitempty"`
	Strategy      string        `json:"strategy"`
	Mode          string        `json:"mode"`
	Waves         []RolloutWave `json:"waves,omitempty"`
	BlockedReason string        `json:"blocked_reason,omitempty"`
}

type RolloutControlStore struct {
	mu       sync.RWMutex
	nextID   int64
	policies map[string]*RolloutPolicy
}

func NewRolloutControlStore() *RolloutControlStore {
	return &RolloutControlStore{policies: map[string]*RolloutPolicy{}}
}

func (s *RolloutControlStore) UpsertPolicy(in RolloutPolicyInput) (RolloutPolicy, error) {
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	if environment == "" {
		return RolloutPolicy{}, errors.New("environment is required")
	}
	strategy := strings.ToLower(strings.TrimSpace(in.Strategy))
	if strategy != "blue-green" && strategy != "canary" && strategy != "rolling" {
		return RolloutPolicy{}, errors.New("strategy must be blue-green, canary, or rolling")
	}
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if mode == "" {
		mode = "serial"
	}
	if mode != "serial" && mode != "batch" && mode != "percentage" {
		return RolloutPolicy{}, errors.New("mode must be serial, batch, or percentage")
	}
	batchSize := in.BatchSize
	if batchSize <= 0 {
		batchSize = 1
	}
	batchPercent := in.BatchPercent
	if batchPercent <= 0 || batchPercent > 100 {
		batchPercent = 25
	}
	canaryPercent := in.CanaryPercent
	if canaryPercent <= 0 || canaryPercent > 100 {
		canaryPercent = 10
	}
	maxUnavailable := in.MaxUnavailable
	if maxUnavailable <= 0 {
		maxUnavailable = 1
	}
	item := RolloutPolicy{
		Environment:    environment,
		Strategy:       strategy,
		Mode:           mode,
		BatchSize:      batchSize,
		BatchPercent:   batchPercent,
		CanaryPercent:  canaryPercent,
		MaxUnavailable: maxUnavailable,
		UpdatedAt:      time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.policies[environment]; ok {
		item.ID = existing.ID
		s.policies[environment] = &item
		return item, nil
	}
	s.nextID++
	item.ID = "rollout-policy-" + itoa(s.nextID)
	s.policies[environment] = &item
	return item, nil
}

func (s *RolloutControlStore) ListPolicies() []RolloutPolicy {
	s.mu.RLock()
	out := make([]RolloutPolicy, 0, len(s.policies))
	for _, item := range s.policies {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Environment < out[j].Environment })
	return out
}

func (s *RolloutControlStore) Plan(in RolloutPlanInput) RolloutPlan {
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	if environment == "" {
		return RolloutPlan{Allowed: false, Environment: environment, BlockedReason: "environment is required"}
	}
	targets := make([]string, 0, len(in.Targets))
	for _, target := range in.Targets {
		t := strings.TrimSpace(target)
		if t != "" {
			targets = append(targets, t)
		}
	}
	if len(targets) == 0 {
		return RolloutPlan{Allowed: false, Environment: environment, BlockedReason: "targets are required"}
	}
	policy, ok := s.policies[environment]
	if !ok {
		policy = &RolloutPolicy{
			Environment:   environment,
			Strategy:      "rolling",
			Mode:          "serial",
			BatchSize:     1,
			BatchPercent:  25,
			CanaryPercent: 10,
		}
	}

	waves := make([]RolloutWave, 0, len(targets))
	switch policy.Strategy {
	case "blue-green":
		blue := append([]string{}, targets...)
		green := append([]string{}, targets...)
		waves = append(waves, RolloutWave{Index: 1, Targets: blue, Phase: "blue-prepare", Reason: "prepare blue environment"})
		waves = append(waves, RolloutWave{Index: 2, Targets: green, Phase: "green-cutover", Reason: "switch traffic to green environment"})
	case "canary":
		size := maxRolloutInt(1, int(float64(len(targets))*float64(policy.CanaryPercent)/100.0))
		if size > len(targets) {
			size = len(targets)
		}
		waves = append(waves, RolloutWave{Index: 1, Targets: append([]string{}, targets[:size]...), Phase: "canary", Reason: "canary wave"})
		if size < len(targets) {
			waves = append(waves, RolloutWave{Index: 2, Targets: append([]string{}, targets[size:]...), Phase: "promote", Reason: "promote after canary"})
		}
	default:
		batch := 1
		switch policy.Mode {
		case "batch":
			batch = policy.BatchSize
		case "percentage":
			batch = maxRolloutInt(1, int(float64(len(targets))*float64(policy.BatchPercent)/100.0))
		default:
			batch = 1
		}
		if batch > len(targets) {
			batch = len(targets)
		}
		waveIdx := 1
		for i := 0; i < len(targets); i += batch {
			end := i + batch
			if end > len(targets) {
				end = len(targets)
			}
			waves = append(waves, RolloutWave{
				Index:   waveIdx,
				Targets: append([]string{}, targets[i:end]...),
				Phase:   "rolling",
				Reason:  "rolling wave using selected rollout mode",
			})
			waveIdx++
		}
	}

	return RolloutPlan{
		Allowed:     true,
		Environment: environment,
		PolicyID:    policy.ID,
		Strategy:    policy.Strategy,
		Mode:        policy.Mode,
		Waves:       waves,
	}
}

func maxRolloutInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
