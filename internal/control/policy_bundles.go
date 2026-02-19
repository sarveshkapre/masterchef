package control

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type PolicyLockEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
	Source  string `json:"source,omitempty"`
}

type VersionedPolicyBundle struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	PolicyGroup string            `json:"policy_group"`
	RunList     []string          `json:"run_list,omitempty"`
	Variables   map[string]string `json:"variables,omitempty"`
	LockEntries []PolicyLockEntry `json:"lock_entries,omitempty"`
	LockDigest  string            `json:"lock_digest"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type PolicyBundleInput struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	PolicyGroup string            `json:"policy_group,omitempty"`
	RunList     []string          `json:"run_list,omitempty"`
	Variables   map[string]string `json:"variables,omitempty"`
	LockEntries []PolicyLockEntry `json:"lock_entries,omitempty"`
}

type PolicyBundlePromotionInput struct {
	TargetGroup string   `json:"target_group"`
	RunList     []string `json:"run_list,omitempty"`
	Reason      string   `json:"reason,omitempty"`
}

type PolicyBundlePromotion struct {
	ID          string    `json:"id"`
	BundleID    string    `json:"bundle_id"`
	BundleName  string    `json:"bundle_name"`
	BundleVer   string    `json:"bundle_version"`
	FromGroup   string    `json:"from_group"`
	TargetGroup string    `json:"target_group"`
	RunList     []string  `json:"run_list,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	PromotedAt  time.Time `json:"promoted_at"`
}

type PolicyBundleStore struct {
	mu            sync.RWMutex
	nextBundleID  int64
	nextPromotion int64
	bundles       map[string]VersionedPolicyBundle
	promotions    map[string][]PolicyBundlePromotion
}

func NewPolicyBundleStore() *PolicyBundleStore {
	return &PolicyBundleStore{
		bundles:    map[string]VersionedPolicyBundle{},
		promotions: map[string][]PolicyBundlePromotion{},
	}
}

func (s *PolicyBundleStore) Create(in PolicyBundleInput) (VersionedPolicyBundle, error) {
	name := strings.TrimSpace(in.Name)
	version := strings.TrimSpace(in.Version)
	if name == "" {
		return VersionedPolicyBundle{}, errors.New("name is required")
	}
	if version == "" {
		return VersionedPolicyBundle{}, errors.New("version is required")
	}
	group := strings.TrimSpace(in.PolicyGroup)
	if group == "" {
		group = "default"
	}
	entries, err := normalizePolicyLockEntries(in.LockEntries)
	if err != nil {
		return VersionedPolicyBundle{}, err
	}
	now := time.Now().UTC()
	bundle := VersionedPolicyBundle{
		Name:        name,
		Version:     version,
		PolicyGroup: group,
		RunList:     normalizeStringSlice(in.RunList),
		Variables:   cloneStringMap(in.Variables),
		LockEntries: entries,
		LockDigest:  policyLockDigest(entries),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextBundleID++
	bundle.ID = "policybundle-" + itoa(s.nextBundleID)
	s.bundles[bundle.ID] = clonePolicyBundle(bundle)
	return clonePolicyBundle(bundle), nil
}

func (s *PolicyBundleStore) List() []VersionedPolicyBundle {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]VersionedPolicyBundle, 0, len(s.bundles))
	for _, bundle := range s.bundles {
		out = append(out, clonePolicyBundle(bundle))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *PolicyBundleStore) Get(id string) (VersionedPolicyBundle, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	bundle, ok := s.bundles[strings.TrimSpace(id)]
	if !ok {
		return VersionedPolicyBundle{}, false
	}
	return clonePolicyBundle(bundle), true
}

func (s *PolicyBundleStore) Promote(bundleID string, in PolicyBundlePromotionInput) (PolicyBundlePromotion, error) {
	bundleID = strings.TrimSpace(bundleID)
	target := strings.TrimSpace(in.TargetGroup)
	if target == "" {
		return PolicyBundlePromotion{}, errors.New("target_group is required")
	}
	reason := strings.TrimSpace(in.Reason)
	s.mu.Lock()
	defer s.mu.Unlock()
	bundle, ok := s.bundles[bundleID]
	if !ok {
		return PolicyBundlePromotion{}, errors.New("policy bundle not found")
	}
	runList := normalizeStringSlice(in.RunList)
	if len(runList) == 0 {
		runList = append([]string{}, bundle.RunList...)
	}
	s.nextPromotion++
	promo := PolicyBundlePromotion{
		ID:          "policypromo-" + itoa(s.nextPromotion),
		BundleID:    bundle.ID,
		BundleName:  bundle.Name,
		BundleVer:   bundle.Version,
		FromGroup:   bundle.PolicyGroup,
		TargetGroup: target,
		RunList:     runList,
		Reason:      reason,
		PromotedAt:  time.Now().UTC(),
	}
	bundle.PolicyGroup = target
	bundle.UpdatedAt = promo.PromotedAt
	if len(runList) > 0 {
		bundle.RunList = append([]string{}, runList...)
	}
	s.bundles[bundleID] = clonePolicyBundle(bundle)
	s.promotions[bundleID] = append(s.promotions[bundleID], promo)
	return clonePolicyPromotion(promo), nil
}

func (s *PolicyBundleStore) ListPromotions(bundleID string) []PolicyBundlePromotion {
	s.mu.RLock()
	defer s.mu.RUnlock()
	bundleID = strings.TrimSpace(bundleID)
	in := s.promotions[bundleID]
	out := make([]PolicyBundlePromotion, 0, len(in))
	for _, promo := range in {
		out = append(out, clonePolicyPromotion(promo))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PromotedAt.After(out[j].PromotedAt) })
	return out
}

func normalizePolicyLockEntries(in []PolicyLockEntry) ([]PolicyLockEntry, error) {
	if len(in) == 0 {
		return nil, nil
	}
	seen := map[string]struct{}{}
	out := make([]PolicyLockEntry, 0, len(in))
	for i, raw := range in {
		item := PolicyLockEntry{
			Name:    strings.TrimSpace(raw.Name),
			Version: strings.TrimSpace(raw.Version),
			Digest:  strings.TrimSpace(raw.Digest),
			Source:  strings.TrimSpace(raw.Source),
		}
		if item.Name == "" {
			return nil, fmt.Errorf("lock_entries[%d].name is required", i)
		}
		if item.Version == "" {
			return nil, fmt.Errorf("lock_entries[%d].version is required", i)
		}
		if item.Digest == "" {
			return nil, fmt.Errorf("lock_entries[%d].digest is required", i)
		}
		key := item.Name + "@" + item.Version
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate lock entry %q", key)
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		li := out[i].Name + "@" + out[i].Version
		lj := out[j].Name + "@" + out[j].Version
		if li == lj {
			return out[i].Digest < out[j].Digest
		}
		return li < lj
	})
	return out, nil
}

func policyLockDigest(entries []PolicyLockEntry) string {
	if len(entries) == 0 {
		return ""
	}
	parts := make([]string, 0, len(entries))
	for _, item := range entries {
		parts = append(parts, item.Name+"|"+item.Version+"|"+item.Digest+"|"+item.Source)
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func clonePolicyBundle(in VersionedPolicyBundle) VersionedPolicyBundle {
	out := in
	out.RunList = append([]string{}, in.RunList...)
	out.Variables = cloneStringMap(in.Variables)
	out.LockEntries = make([]PolicyLockEntry, 0, len(in.LockEntries))
	for _, item := range in.LockEntries {
		out.LockEntries = append(out.LockEntries, item)
	}
	return out
}

func clonePolicyPromotion(in PolicyBundlePromotion) PolicyBundlePromotion {
	out := in
	out.RunList = append([]string{}, in.RunList...)
	return out
}
