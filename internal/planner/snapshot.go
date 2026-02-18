package planner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type PlanSnapshot struct {
	Version string `json:"version"`
	Plan    *Plan  `json:"plan"`
}

type PlanSnapshotDiff struct {
	Match        bool     `json:"match"`
	AddedSteps   []string `json:"added_steps,omitempty"`
	RemovedSteps []string `json:"removed_steps,omitempty"`
	ChangedSteps []string `json:"changed_steps,omitempty"`
	BaselineHash string   `json:"baseline_hash"`
	CurrentHash  string   `json:"current_hash"`
}

func SaveSnapshot(path string, p *Plan) error {
	snap := PlanSnapshot{
		Version: "v1",
		Plan:    p,
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func LoadSnapshot(path string) (PlanSnapshot, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return PlanSnapshot{}, err
	}
	var snap PlanSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return PlanSnapshot{}, err
	}
	if snap.Plan == nil {
		snap.Plan = &Plan{}
	}
	return snap, nil
}

func CompareSnapshot(path string, current *Plan) (PlanSnapshotDiff, error) {
	snap, err := LoadSnapshot(path)
	if err != nil {
		return PlanSnapshotDiff{}, err
	}
	return DiffPlans(snap.Plan, current), nil
}

func DiffPlans(baseline, current *Plan) PlanSnapshotDiff {
	if baseline == nil {
		baseline = &Plan{}
	}
	if current == nil {
		current = &Plan{}
	}
	baseMap := stepFingerprints(baseline)
	curMap := stepFingerprints(current)

	added := make([]string, 0)
	removed := make([]string, 0)
	changed := make([]string, 0)

	for id, curFP := range curMap {
		baseFP, ok := baseMap[id]
		if !ok {
			added = append(added, id)
			continue
		}
		if curFP != baseFP {
			changed = append(changed, id)
		}
	}
	for id := range baseMap {
		if _, ok := curMap[id]; !ok {
			removed = append(removed, id)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(changed)

	return PlanSnapshotDiff{
		Match:        len(added) == 0 && len(removed) == 0 && len(changed) == 0,
		AddedSteps:   added,
		RemovedSteps: removed,
		ChangedSteps: changed,
		BaselineHash: hashPlan(baseline),
		CurrentHash:  hashPlan(current),
	}
}

func stepFingerprints(p *Plan) map[string]string {
	out := map[string]string{}
	for _, step := range p.Steps {
		id := strings.TrimSpace(step.Resource.ID)
		if id == "" {
			continue
		}
		b, _ := json.Marshal(step)
		sum := sha256.Sum256(b)
		out[id] = hex.EncodeToString(sum[:])
	}
	return out
}

func hashPlan(p *Plan) string {
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
