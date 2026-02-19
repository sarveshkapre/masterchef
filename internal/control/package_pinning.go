package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type PackagePinPolicyInput struct {
	TargetKind   string `json:"target_kind"`
	Target       string `json:"target"`
	Package      string `json:"package"`
	Version      string `json:"version"`
	Held         bool   `json:"held"`
	EnforceDrift bool   `json:"enforce_drift"`
}

type PackagePinPolicy struct {
	ID           string    `json:"id"`
	TargetKind   string    `json:"target_kind"`
	Target       string    `json:"target"`
	Package      string    `json:"package"`
	Version      string    `json:"version"`
	Held         bool      `json:"held"`
	EnforceDrift bool      `json:"enforce_drift"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type PackagePinEvaluateInput struct {
	TargetKind       string `json:"target_kind"`
	Target           string `json:"target"`
	Package          string `json:"package"`
	InstalledVersion string `json:"installed_version,omitempty"`
	HoldApplied      bool   `json:"hold_applied"`
}

type PackagePinDecision struct {
	Allowed          bool   `json:"allowed"`
	PolicyID         string `json:"policy_id,omitempty"`
	Package          string `json:"package"`
	DesiredVersion   string `json:"desired_version,omitempty"`
	InstalledVersion string `json:"installed_version,omitempty"`
	Action           string `json:"action"` // noop|install|upgrade|downgrade|hold|unhold|block
	DriftDetected    bool   `json:"drift_detected"`
	Reason           string `json:"reason"`
}

type PackagePinStore struct {
	mu       sync.RWMutex
	nextID   int64
	policies map[string]*PackagePinPolicy
}

func NewPackagePinStore() *PackagePinStore {
	return &PackagePinStore{policies: map[string]*PackagePinPolicy{}}
}

func (s *PackagePinStore) Upsert(in PackagePinPolicyInput) (PackagePinPolicy, error) {
	targetKind := strings.ToLower(strings.TrimSpace(in.TargetKind))
	target := strings.ToLower(strings.TrimSpace(in.Target))
	pkg := strings.ToLower(strings.TrimSpace(in.Package))
	version := strings.TrimSpace(in.Version)
	if targetKind == "" || target == "" || pkg == "" || version == "" {
		return PackagePinPolicy{}, errors.New("target_kind, target, package, and version are required")
	}
	item := PackagePinPolicy{
		TargetKind:   targetKind,
		Target:       target,
		Package:      pkg,
		Version:      version,
		Held:         in.Held,
		EnforceDrift: in.EnforceDrift,
		UpdatedAt:    time.Now().UTC(),
	}
	key := packagePinKey(targetKind, target, pkg)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.policies[key]; ok {
		item.ID = existing.ID
		s.policies[key] = &item
		return item, nil
	}
	s.nextID++
	item.ID = "package-pin-policy-" + itoa(s.nextID)
	s.policies[key] = &item
	return item, nil
}

func (s *PackagePinStore) List() []PackagePinPolicy {
	s.mu.RLock()
	out := make([]PackagePinPolicy, 0, len(s.policies))
	for _, item := range s.policies {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].TargetKind == out[j].TargetKind {
			if out[i].Target == out[j].Target {
				return out[i].Package < out[j].Package
			}
			return out[i].Target < out[j].Target
		}
		return out[i].TargetKind < out[j].TargetKind
	})
	return out
}

func (s *PackagePinStore) Evaluate(in PackagePinEvaluateInput) PackagePinDecision {
	targetKind := strings.ToLower(strings.TrimSpace(in.TargetKind))
	target := strings.ToLower(strings.TrimSpace(in.Target))
	pkg := strings.ToLower(strings.TrimSpace(in.Package))
	installed := strings.TrimSpace(in.InstalledVersion)
	if targetKind == "" || target == "" || pkg == "" {
		return PackagePinDecision{
			Allowed: false,
			Package: pkg,
			Action:  "block",
			Reason:  "target_kind, target, and package are required",
		}
	}

	key := packagePinKey(targetKind, target, pkg)
	s.mu.RLock()
	item, ok := s.policies[key]
	s.mu.RUnlock()
	if !ok {
		return PackagePinDecision{
			Allowed:          true,
			Package:          pkg,
			InstalledVersion: installed,
			Action:           "noop",
			Reason:           "no pin policy configured",
		}
	}

	decision := PackagePinDecision{
		Allowed:          true,
		PolicyID:         item.ID,
		Package:          pkg,
		DesiredVersion:   item.Version,
		InstalledVersion: installed,
		Action:           "noop",
		Reason:           "package version already compliant",
	}
	if installed == "" {
		decision.Action = "install"
		decision.Reason = "package missing; install pinned version"
	}
	if installed != "" && installed != item.Version {
		decision.DriftDetected = true
		if item.EnforceDrift {
			if installed > item.Version {
				decision.Action = "downgrade"
				decision.Reason = "installed version exceeds pinned version; enforce downgrade"
			} else {
				decision.Action = "upgrade"
				decision.Reason = "installed version below pinned version; enforce upgrade"
			}
		} else {
			decision.Action = "noop"
			decision.Reason = "drift detected but enforcement is disabled"
		}
	}
	if item.Held && !in.HoldApplied {
		decision.Action = "hold"
		decision.Reason = "apply package hold to lock pinned version"
	}
	if !item.Held && in.HoldApplied {
		decision.Action = "unhold"
		decision.Reason = "remove package hold to follow policy"
	}
	return decision
}

func packagePinKey(targetKind, target, pkg string) string {
	return targetKind + "|" + target + "|" + pkg
}
