package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type CompatibilityShimInput struct {
	SourcePlatform  string   `json:"source_platform"` // chef|ansible|puppet|salt|generic
	LegacyPattern   string   `json:"legacy_pattern"`
	Description     string   `json:"description"`
	Target          string   `json:"target"`
	Keywords        []string `json:"keywords,omitempty"`
	RiskLevel       string   `json:"risk_level,omitempty"` // low|medium|high
	Recommendation  string   `json:"recommendation,omitempty"`
	ConvergenceSafe bool     `json:"convergence_safe"`
}

type CompatibilityShim struct {
	ID              string    `json:"id"`
	SourcePlatform  string    `json:"source_platform"`
	LegacyPattern   string    `json:"legacy_pattern"`
	Description     string    `json:"description"`
	Target          string    `json:"target"`
	Keywords        []string  `json:"keywords,omitempty"`
	RiskLevel       string    `json:"risk_level"`
	Recommendation  string    `json:"recommendation,omitempty"`
	ConvergenceSafe bool      `json:"convergence_safe"`
	Enabled         bool      `json:"enabled"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type CompatibilityShimResolveInput struct {
	SourcePlatform  string `json:"source_platform,omitempty"`
	Pattern         string `json:"pattern,omitempty"`
	Content         string `json:"content,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	IncludeDisabled bool   `json:"include_disabled,omitempty"`
}

type CompatibilityShimMatch struct {
	Shim    CompatibilityShim `json:"shim"`
	Score   int               `json:"score"`
	Reasons []string          `json:"reasons,omitempty"`
}

type CompatibilityShimResolveResult struct {
	SourcePlatform  string                   `json:"source_platform,omitempty"`
	Query           string                   `json:"query"`
	Matched         []CompatibilityShimMatch `json:"matched,omitempty"`
	CoverageScore   int                      `json:"coverage_score"`
	Recommendations []string                 `json:"recommendations,omitempty"`
	ResolvedAt      time.Time                `json:"resolved_at"`
}

type CompatibilityShimStore struct {
	mu     sync.RWMutex
	nextID int64
	shims  map[string]*CompatibilityShim
}

func NewCompatibilityShimStore() *CompatibilityShimStore {
	s := &CompatibilityShimStore{
		shims: map[string]*CompatibilityShim{},
	}
	s.seedBuiltins()
	return s
}

func (s *CompatibilityShimStore) Upsert(in CompatibilityShimInput) (CompatibilityShim, error) {
	platform := normalizeFeature(in.SourcePlatform)
	if platform == "" {
		return CompatibilityShim{}, errors.New("source_platform is required")
	}
	if platform != "chef" && platform != "ansible" && platform != "puppet" && platform != "salt" && platform != "generic" {
		return CompatibilityShim{}, errors.New("source_platform must be chef, ansible, puppet, salt, or generic")
	}
	pattern := strings.TrimSpace(in.LegacyPattern)
	if pattern == "" {
		return CompatibilityShim{}, errors.New("legacy_pattern is required")
	}
	description := strings.TrimSpace(in.Description)
	if description == "" {
		return CompatibilityShim{}, errors.New("description is required")
	}
	target := strings.TrimSpace(in.Target)
	if target == "" {
		return CompatibilityShim{}, errors.New("target is required")
	}
	risk := strings.ToLower(strings.TrimSpace(in.RiskLevel))
	if risk == "" {
		risk = "medium"
	}
	if risk != "low" && risk != "medium" && risk != "high" {
		return CompatibilityShim{}, errors.New("risk_level must be low, medium, or high")
	}
	keywords := normalizeStringList(in.Keywords)

	s.mu.Lock()
	defer s.mu.Unlock()
	for id, existing := range s.shims {
		if existing.SourcePlatform == platform && strings.EqualFold(existing.LegacyPattern, pattern) {
			existing.Description = description
			existing.Target = target
			existing.Keywords = keywords
			existing.RiskLevel = risk
			existing.Recommendation = strings.TrimSpace(in.Recommendation)
			existing.ConvergenceSafe = in.ConvergenceSafe
			existing.UpdatedAt = time.Now().UTC()
			s.shims[id] = existing
			return cloneCompatibilityShim(*existing), nil
		}
	}
	s.nextID++
	item := CompatibilityShim{
		ID:              "compat-shim-" + itoa(s.nextID),
		SourcePlatform:  platform,
		LegacyPattern:   pattern,
		Description:     description,
		Target:          target,
		Keywords:        keywords,
		RiskLevel:       risk,
		Recommendation:  strings.TrimSpace(in.Recommendation),
		ConvergenceSafe: in.ConvergenceSafe,
		Enabled:         true,
		UpdatedAt:       time.Now().UTC(),
	}
	s.shims[item.ID] = &item
	return cloneCompatibilityShim(item), nil
}

func (s *CompatibilityShimStore) List() []CompatibilityShim {
	s.mu.RLock()
	out := make([]CompatibilityShim, 0, len(s.shims))
	for _, item := range s.shims {
		out = append(out, cloneCompatibilityShim(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].SourcePlatform != out[j].SourcePlatform {
			return out[i].SourcePlatform < out[j].SourcePlatform
		}
		return out[i].LegacyPattern < out[j].LegacyPattern
	})
	return out
}

func (s *CompatibilityShimStore) Get(id string) (CompatibilityShim, bool) {
	s.mu.RLock()
	item, ok := s.shims[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return CompatibilityShim{}, false
	}
	return cloneCompatibilityShim(*item), true
}

func (s *CompatibilityShimStore) Enable(id string) (CompatibilityShim, error) {
	return s.setEnabled(id, true)
}

func (s *CompatibilityShimStore) Disable(id string) (CompatibilityShim, error) {
	return s.setEnabled(id, false)
}

func (s *CompatibilityShimStore) Resolve(in CompatibilityShimResolveInput) CompatibilityShimResolveResult {
	platform := normalizeFeature(in.SourcePlatform)
	query := strings.ToLower(strings.TrimSpace(in.Pattern + " " + in.Content))
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	s.mu.RLock()
	candidates := make([]CompatibilityShim, 0, len(s.shims))
	for _, item := range s.shims {
		if platform != "" && item.SourcePlatform != platform {
			continue
		}
		if !in.IncludeDisabled && !item.Enabled {
			continue
		}
		candidates = append(candidates, cloneCompatibilityShim(*item))
	}
	s.mu.RUnlock()

	out := CompatibilityShimResolveResult{
		SourcePlatform: platform,
		Query:          strings.TrimSpace(in.Pattern + " " + in.Content),
		ResolvedAt:     time.Now().UTC(),
	}
	matches := make([]CompatibilityShimMatch, 0, len(candidates))
	for _, shim := range candidates {
		score := 0
		reasons := make([]string, 0, 4)
		if query == "" {
			score = 1
			reasons = append(reasons, "no query supplied; included by platform filter")
		}
		pattern := strings.ToLower(strings.TrimSpace(shim.LegacyPattern))
		if query != "" && pattern != "" && strings.Contains(query, pattern) {
			score += 60
			reasons = append(reasons, "query matched legacy pattern")
		}
		for _, keyword := range shim.Keywords {
			k := strings.ToLower(strings.TrimSpace(keyword))
			if k == "" {
				continue
			}
			if strings.Contains(query, k) {
				score += 15
				reasons = append(reasons, "matched keyword: "+k)
			}
		}
		target := strings.ToLower(strings.TrimSpace(shim.Target))
		if query != "" && target != "" && strings.Contains(query, target) {
			score += 10
			reasons = append(reasons, "query referenced target mapping")
		}
		if score <= 0 {
			continue
		}
		if score > 100 {
			score = 100
		}
		matches = append(matches, CompatibilityShimMatch{
			Shim:    shim,
			Score:   score,
			Reasons: reasons,
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Shim.ID < matches[j].Shim.ID
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	out.Matched = matches
	if len(matches) == 0 {
		out.Recommendations = []string{
			"no direct compatibility shim match found",
			"add a custom shim or provider module for this legacy behavior",
		}
		return out
	}
	best := matches[0].Score
	if best > 100 {
		best = 100
	}
	out.CoverageScore = best
	seen := map[string]struct{}{}
	for _, match := range matches {
		rec := strings.TrimSpace(match.Shim.Recommendation)
		if rec == "" {
			continue
		}
		if _, ok := seen[rec]; ok {
			continue
		}
		seen[rec] = struct{}{}
		out.Recommendations = append(out.Recommendations, rec)
	}
	if len(out.Recommendations) == 0 {
		out.Recommendations = []string{"validate matched compatibility shim behavior in check/noop mode before apply"}
	}
	return out
}

func (s *CompatibilityShimStore) setEnabled(id string, enabled bool) (CompatibilityShim, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return CompatibilityShim{}, errors.New("id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.shims[id]
	if !ok {
		return CompatibilityShim{}, errors.New("compatibility shim not found")
	}
	item.Enabled = enabled
	item.UpdatedAt = time.Now().UTC()
	return cloneCompatibilityShim(*item), nil
}

func cloneCompatibilityShim(in CompatibilityShim) CompatibilityShim {
	out := in
	out.Keywords = append([]string{}, in.Keywords...)
	return out
}

func (s *CompatibilityShimStore) seedBuiltins() {
	builtins := []CompatibilityShimInput{
		{
			SourcePlatform:  "ansible",
			LegacyPattern:   "with_items loops",
			Description:     "Translate legacy with_items loops into explicit matrix expansions and stable item keys.",
			Target:          "matrix expansion",
			Keywords:        []string{"with_items", "loop", "item"},
			RiskLevel:       "low",
			Recommendation:  "replace implicit loop context with explicit matrix keys and deterministic item ordering",
			ConvergenceSafe: true,
		},
		{
			SourcePlatform:  "ansible",
			LegacyPattern:   "register + when chaining",
			Description:     "Map register/when task chains to explicit guards and dependency edges.",
			Target:          "guards + requires",
			Keywords:        []string{"register", "when", "changed"},
			RiskLevel:       "medium",
			Recommendation:  "convert chained register conditions into explicit only_if/unless guards",
			ConvergenceSafe: true,
		},
		{
			SourcePlatform:  "chef",
			LegacyPattern:   "search-based dynamic discovery",
			Description:     "Map Chef search patterns to exported resources and resource collectors.",
			Target:          "exported resources",
			Keywords:        []string{"search(", "data_bag", "node["},
			RiskLevel:       "medium",
			Recommendation:  "replace runtime search with exported resource declarations and typed selectors",
			ConvergenceSafe: true,
		},
		{
			SourcePlatform:  "puppet",
			LegacyPattern:   "hiera implicit merge order",
			Description:     "Pin implicit Hiera merges to explicit merge-first/merge-last/overwrite policies.",
			Target:          "pillar merge policy",
			Keywords:        []string{"hiera", "lookup(", "merge"},
			RiskLevel:       "high",
			Recommendation:  "define explicit merge strategy to remove implicit precedence ambiguity",
			ConvergenceSafe: true,
		},
		{
			SourcePlatform:  "salt",
			LegacyPattern:   "state.apply orchestration wrappers",
			Description:     "Map state.apply wrappers into task/plan executions with explicit strategy and concurrency controls.",
			Target:          "task plan",
			Keywords:        []string{"state.apply", "orchestrate", "highstate"},
			RiskLevel:       "medium",
			Recommendation:  "port orchestration wrappers into task plans with serial strategy by failure domain",
			ConvergenceSafe: true,
		},
	}
	for _, item := range builtins {
		_, _ = s.Upsert(item)
	}
}
