package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type PatchPolicyInput struct {
	Environment            string   `json:"environment"`
	WindowStartHourUTC     int      `json:"window_start_hour_utc"`
	WindowDurationHours    int      `json:"window_duration_hours"`
	MaxParallelHosts       int      `json:"max_parallel_hosts"`
	AllowedClassifications []string `json:"allowed_classifications,omitempty"` // security|critical|bugfix|feature
	RequireRebootApproval  bool     `json:"require_reboot_approval"`
}

type PatchPolicy struct {
	ID                     string    `json:"id"`
	Environment            string    `json:"environment"`
	WindowStartHourUTC     int       `json:"window_start_hour_utc"`
	WindowDurationHours    int       `json:"window_duration_hours"`
	MaxParallelHosts       int       `json:"max_parallel_hosts"`
	AllowedClassifications []string  `json:"allowed_classifications,omitempty"`
	RequireRebootApproval  bool      `json:"require_reboot_approval"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type PatchHost struct {
	ID             string `json:"id"`
	Classification string `json:"classification"`
	NeedsReboot    bool   `json:"needs_reboot"`
}

type PatchPlanInput struct {
	Environment    string      `json:"environment"`
	HourUTC        int         `json:"hour_utc"`
	Hosts          []PatchHost `json:"hosts"`
	RebootApproved bool        `json:"reboot_approved"`
}

type PatchWave struct {
	Index          int      `json:"index"`
	HostIDs        []string `json:"host_ids"`
	Classification string   `json:"classification"`
	Reason         string   `json:"reason"`
}

type PatchPlan struct {
	Allowed              bool        `json:"allowed"`
	Environment          string      `json:"environment"`
	PolicyID             string      `json:"policy_id,omitempty"`
	WindowOpen           bool        `json:"window_open"`
	BlockedReason        string      `json:"blocked_reason,omitempty"`
	RebootApprovalNeeded bool        `json:"reboot_approval_needed"`
	Waves                []PatchWave `json:"waves,omitempty"`
}

type PatchManagementStore struct {
	mu       sync.RWMutex
	nextID   int64
	policies map[string]*PatchPolicy
}

func NewPatchManagementStore() *PatchManagementStore {
	return &PatchManagementStore{policies: map[string]*PatchPolicy{}}
}

func (s *PatchManagementStore) UpsertPolicy(in PatchPolicyInput) (PatchPolicy, error) {
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	if environment == "" {
		return PatchPolicy{}, errors.New("environment is required")
	}
	start := in.WindowStartHourUTC
	if start < 0 || start > 23 {
		return PatchPolicy{}, errors.New("window_start_hour_utc must be between 0 and 23")
	}
	duration := in.WindowDurationHours
	if duration <= 0 || duration > 24 {
		return PatchPolicy{}, errors.New("window_duration_hours must be between 1 and 24")
	}
	maxParallel := in.MaxParallelHosts
	if maxParallel <= 0 {
		maxParallel = 1
	}
	item := PatchPolicy{
		Environment:            environment,
		WindowStartHourUTC:     start,
		WindowDurationHours:    duration,
		MaxParallelHosts:       maxParallel,
		AllowedClassifications: normalizeStringList(in.AllowedClassifications),
		RequireRebootApproval:  in.RequireRebootApproval,
		UpdatedAt:              time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.policies[environment]; ok {
		item.ID = existing.ID
		s.policies[environment] = &item
		return item, nil
	}
	s.nextID++
	item.ID = "patch-policy-" + itoa(s.nextID)
	s.policies[environment] = &item
	return item, nil
}

func (s *PatchManagementStore) ListPolicies() []PatchPolicy {
	s.mu.RLock()
	out := make([]PatchPolicy, 0, len(s.policies))
	for _, item := range s.policies {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Environment < out[j].Environment })
	return out
}

func (s *PatchManagementStore) Plan(in PatchPlanInput) PatchPlan {
	environment := strings.ToLower(strings.TrimSpace(in.Environment))
	if environment == "" {
		return PatchPlan{Allowed: false, Environment: environment, BlockedReason: "environment is required"}
	}
	if len(in.Hosts) == 0 {
		return PatchPlan{Allowed: false, Environment: environment, BlockedReason: "hosts are required"}
	}
	policy, ok := s.policies[environment]
	if !ok {
		policy = &PatchPolicy{
			Environment:            environment,
			WindowStartHourUTC:     0,
			WindowDurationHours:    24,
			MaxParallelHosts:       1,
			AllowedClassifications: []string{"security", "critical", "bugfix", "feature"},
		}
	}
	windowOpen := hourInPatchWindow(in.HourUTC, policy.WindowStartHourUTC, policy.WindowDurationHours)
	if !windowOpen {
		return PatchPlan{
			Allowed:       false,
			Environment:   environment,
			PolicyID:      policy.ID,
			WindowOpen:    false,
			BlockedReason: "patch window is closed",
		}
	}

	waves := make([]PatchWave, 0, len(in.Hosts))
	filtered := make([]PatchHost, 0, len(in.Hosts))
	allowed := map[string]struct{}{}
	for _, c := range policy.AllowedClassifications {
		allowed[c] = struct{}{}
	}
	rebootNeeded := false
	for _, host := range in.Hosts {
		classification := strings.ToLower(strings.TrimSpace(host.Classification))
		if classification == "" {
			classification = "security"
		}
		if len(allowed) > 0 {
			if _, ok := allowed[classification]; !ok {
				continue
			}
		}
		host.Classification = classification
		if host.NeedsReboot {
			rebootNeeded = true
		}
		filtered = append(filtered, host)
	}
	if len(filtered) == 0 {
		return PatchPlan{
			Allowed:       false,
			Environment:   environment,
			PolicyID:      policy.ID,
			WindowOpen:    true,
			BlockedReason: "no hosts matched allowed patch classifications",
		}
	}
	if rebootNeeded && policy.RequireRebootApproval && !in.RebootApproved {
		return PatchPlan{
			Allowed:              false,
			Environment:          environment,
			PolicyID:             policy.ID,
			WindowOpen:           true,
			RebootApprovalNeeded: true,
			BlockedReason:        "reboot approval is required for selected host set",
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Classification == filtered[j].Classification {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].Classification < filtered[j].Classification
	})
	wave := 1
	for i := 0; i < len(filtered); i += policy.MaxParallelHosts {
		end := i + policy.MaxParallelHosts
		if end > len(filtered) {
			end = len(filtered)
		}
		hostIDs := make([]string, 0, end-i)
		for _, host := range filtered[i:end] {
			hostIDs = append(hostIDs, host.ID)
		}
		waves = append(waves, PatchWave{
			Index:          wave,
			HostIDs:        hostIDs,
			Classification: filtered[i].Classification,
			Reason:         "scheduled patch wave respecting max parallel hosts",
		})
		wave++
	}
	return PatchPlan{
		Allowed:              true,
		Environment:          environment,
		PolicyID:             policy.ID,
		WindowOpen:           true,
		RebootApprovalNeeded: rebootNeeded && policy.RequireRebootApproval,
		Waves:                waves,
	}
}

func hourInPatchWindow(hour, start, duration int) bool {
	if hour < 0 || hour > 23 {
		return false
	}
	if duration >= 24 {
		return true
	}
	end := (start + duration) % 24
	if start < end {
		return hour >= start && hour < end
	}
	return hour >= start || hour < end
}
