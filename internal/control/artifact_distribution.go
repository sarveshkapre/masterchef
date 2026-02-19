package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ArtifactDistributionPolicyInput struct {
	Environment             string `json:"environment"`
	MaxTransferMbps         int    `json:"max_transfer_mbps"`
	CacheWarmThresholdMB    int    `json:"cache_warm_threshold_mb"`
	PreferRegionalCache     bool   `json:"prefer_regional_cache"`
	RelayOnConstrainedLinks bool   `json:"relay_on_constrained_links"`
	Compression             string `json:"compression,omitempty"`
}

type ArtifactDistributionPolicy struct {
	ID                      string    `json:"id"`
	Environment             string    `json:"environment"`
	MaxTransferMbps         int       `json:"max_transfer_mbps"`
	CacheWarmThresholdMB    int       `json:"cache_warm_threshold_mb"`
	PreferRegionalCache     bool      `json:"prefer_regional_cache"`
	RelayOnConstrainedLinks bool      `json:"relay_on_constrained_links"`
	Compression             string    `json:"compression"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type ArtifactDistributionPlanInput struct {
	Environment            string `json:"environment"`
	ArtifactID             string `json:"artifact_id"`
	ArtifactSizeMB         int    `json:"artifact_size_mb"`
	AvailableBandwidthMbps int    `json:"available_bandwidth_mbps"`
	CacheHit               bool   `json:"cache_hit,omitempty"`
	Urgent                 bool   `json:"urgent,omitempty"`
}

type ArtifactDistributionPlan struct {
	Allowed                  bool   `json:"allowed"`
	Environment              string `json:"environment"`
	PolicyID                 string `json:"policy_id,omitempty"`
	Strategy                 string `json:"strategy"`
	ThrottleMbps             int    `json:"throttle_mbps,omitempty"`
	UseCompression           bool   `json:"use_compression"`
	Compression              string `json:"compression,omitempty"`
	PreWarmCache             bool   `json:"pre_warm_cache"`
	EstimatedTransferSeconds int    `json:"estimated_transfer_seconds,omitempty"`
	Reason                   string `json:"reason"`
}

type ArtifactDistributionStore struct {
	mu       sync.RWMutex
	nextID   int64
	policies map[string]*ArtifactDistributionPolicy
	byEnv    map[string]string
}

func NewArtifactDistributionStore() *ArtifactDistributionStore {
	return &ArtifactDistributionStore{
		policies: map[string]*ArtifactDistributionPolicy{},
		byEnv:    map[string]string{},
	}
}

func (s *ArtifactDistributionStore) Upsert(in ArtifactDistributionPolicyInput) (ArtifactDistributionPolicy, error) {
	env := strings.ToLower(strings.TrimSpace(in.Environment))
	if env == "" {
		return ArtifactDistributionPolicy{}, errors.New("environment is required")
	}
	if in.MaxTransferMbps <= 0 {
		return ArtifactDistributionPolicy{}, errors.New("max_transfer_mbps must be greater than zero")
	}
	threshold := in.CacheWarmThresholdMB
	if threshold <= 0 {
		threshold = 100
	}
	compression := strings.ToLower(strings.TrimSpace(in.Compression))
	if compression == "" {
		compression = "zstd"
	}
	if compression != "zstd" && compression != "gzip" && compression != "none" {
		return ArtifactDistributionPolicy{}, errors.New("compression must be one of: zstd, gzip, none")
	}
	item := ArtifactDistributionPolicy{
		Environment:             env,
		MaxTransferMbps:         in.MaxTransferMbps,
		CacheWarmThresholdMB:    threshold,
		PreferRegionalCache:     in.PreferRegionalCache,
		RelayOnConstrainedLinks: in.RelayOnConstrainedLinks,
		Compression:             compression,
		UpdatedAt:               time.Now().UTC(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.byEnv[env]; ok {
		item.ID = existing
		s.policies[existing] = &item
		return item, nil
	}
	s.nextID++
	item.ID = "artifact-policy-" + itoa(s.nextID)
	s.policies[item.ID] = &item
	s.byEnv[env] = item.ID
	return item, nil
}

func (s *ArtifactDistributionStore) List() []ArtifactDistributionPolicy {
	s.mu.RLock()
	out := make([]ArtifactDistributionPolicy, 0, len(s.policies))
	for _, item := range s.policies {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Environment < out[j].Environment })
	return out
}

func (s *ArtifactDistributionStore) policyByEnvironment(environment string) (ArtifactDistributionPolicy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byEnv[strings.ToLower(strings.TrimSpace(environment))]
	if !ok {
		return ArtifactDistributionPolicy{}, false
	}
	item, ok := s.policies[id]
	if !ok {
		return ArtifactDistributionPolicy{}, false
	}
	return *item, true
}

func (s *ArtifactDistributionStore) Plan(in ArtifactDistributionPlanInput) (ArtifactDistributionPlan, error) {
	env := strings.ToLower(strings.TrimSpace(in.Environment))
	if env == "" {
		return ArtifactDistributionPlan{}, errors.New("environment is required")
	}
	if strings.TrimSpace(in.ArtifactID) == "" {
		return ArtifactDistributionPlan{}, errors.New("artifact_id is required")
	}
	if in.ArtifactSizeMB <= 0 {
		return ArtifactDistributionPlan{}, errors.New("artifact_size_mb must be greater than zero")
	}
	if in.AvailableBandwidthMbps <= 0 {
		return ArtifactDistributionPlan{
			Allowed:     false,
			Environment: env,
			Strategy:    "defer",
			Reason:      "no available bandwidth",
		}, nil
	}

	policy, ok := s.policyByEnvironment(env)
	if !ok {
		seconds := estimateTransferSeconds(in.ArtifactSizeMB, in.AvailableBandwidthMbps)
		return ArtifactDistributionPlan{
			Allowed:                  true,
			Environment:              env,
			Strategy:                 "direct",
			UseCompression:           true,
			Compression:              "zstd",
			EstimatedTransferSeconds: seconds,
			Reason:                   "no distribution policy configured",
		}, nil
	}

	decision := ArtifactDistributionPlan{
		Allowed:        true,
		Environment:    env,
		PolicyID:       policy.ID,
		Strategy:       "direct",
		UseCompression: policy.Compression != "none",
		Compression:    policy.Compression,
		Reason:         "direct transfer within policy limits",
	}

	if in.CacheHit {
		decision.Strategy = "regional-cache"
		decision.PreWarmCache = false
		decision.Reason = "cache hit available in region"
	}
	if policy.PreferRegionalCache && in.ArtifactSizeMB >= policy.CacheWarmThresholdMB {
		decision.Strategy = "regional-cache"
		decision.PreWarmCache = true
		decision.Reason = "large artifact routed through cache tier"
	}
	if policy.RelayOnConstrainedLinks && in.AvailableBandwidthMbps < 20 {
		decision.Strategy = "relay-cache"
		decision.Reason = "constrained link routed through relay cache"
	}

	targetMbps := policy.MaxTransferMbps
	if targetMbps <= 0 || targetMbps > in.AvailableBandwidthMbps {
		targetMbps = in.AvailableBandwidthMbps
	}
	if in.Urgent {
		targetMbps = in.AvailableBandwidthMbps
		if decision.Strategy == "relay-cache" {
			decision.Strategy = "direct"
		}
		decision.Reason = "urgent artifact bypassed throttling"
	}
	decision.ThrottleMbps = targetMbps
	decision.EstimatedTransferSeconds = estimateTransferSeconds(in.ArtifactSizeMB, targetMbps)
	return decision, nil
}

func estimateTransferSeconds(sizeMB, bandwidthMbps int) int {
	if sizeMB <= 0 || bandwidthMbps <= 0 {
		return 0
	}
	bits := float64(sizeMB) * 8.0
	seconds := bits / float64(bandwidthMbps)
	if seconds < 1 {
		return 1
	}
	return int(seconds + 0.999)
}
