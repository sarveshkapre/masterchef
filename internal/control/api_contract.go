package control

import (
	"sort"
)

type APISpec struct {
	Version   string   `json:"version"`
	Endpoints []string `json:"endpoints"`
}

type APIDiffReport struct {
	BaselineVersion    string   `json:"baseline_version"`
	CurrentVersion     string   `json:"current_version"`
	Added              []string `json:"added"`
	Removed            []string `json:"removed"`
	Unchanged          []string `json:"unchanged"`
	BackwardCompatible bool     `json:"backward_compatible"`
	ForwardCompatible  bool     `json:"forward_compatible"`
}

func DiffAPISpec(baseline, current APISpec) APIDiffReport {
	baseSet := map[string]struct{}{}
	curSet := map[string]struct{}{}
	for _, e := range baseline.Endpoints {
		baseSet[e] = struct{}{}
	}
	for _, e := range current.Endpoints {
		curSet[e] = struct{}{}
	}

	added := make([]string, 0)
	removed := make([]string, 0)
	unchanged := make([]string, 0)

	for e := range curSet {
		if _, ok := baseSet[e]; ok {
			unchanged = append(unchanged, e)
		} else {
			added = append(added, e)
		}
	}
	for e := range baseSet {
		if _, ok := curSet[e]; !ok {
			removed = append(removed, e)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(unchanged)

	return APIDiffReport{
		BaselineVersion:    baseline.Version,
		CurrentVersion:     current.Version,
		Added:              added,
		Removed:            removed,
		Unchanged:          unchanged,
		BackwardCompatible: len(removed) == 0,
		ForwardCompatible:  len(added) == 0,
	}
}
