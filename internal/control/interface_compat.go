package control

import (
	"errors"
	"sort"
	"strings"
)

type InterfaceContract struct {
	Kind    string            `json:"kind"` // module|provider
	Name    string            `json:"name"`
	Version string            `json:"version"`
	Inputs  map[string]string `json:"inputs,omitempty"`  // name -> type
	Outputs map[string]string `json:"outputs,omitempty"` // name -> type
}

type InterfaceChange struct {
	Field       string `json:"field"`
	BeforeType  string `json:"before_type,omitempty"`
	AfterType   string `json:"after_type,omitempty"`
	Description string `json:"description"`
}

type InterfaceCompatReport struct {
	Kind             string            `json:"kind"`
	Name             string            `json:"name"`
	FromVersion      string            `json:"from_version"`
	ToVersion        string            `json:"to_version"`
	Breaking         bool              `json:"breaking"`
	BreakingChanges  []InterfaceChange `json:"breaking_changes,omitempty"`
	NonBreakingHints []string          `json:"non_breaking_hints,omitempty"`
}

func DetectInterfaceCompatibility(baseline, current InterfaceContract) (InterfaceCompatReport, error) {
	kind := strings.ToLower(strings.TrimSpace(current.Kind))
	if kind != "module" && kind != "provider" {
		return InterfaceCompatReport{}, errors.New("kind must be module or provider")
	}
	name := strings.TrimSpace(current.Name)
	if name == "" || !strings.EqualFold(strings.TrimSpace(baseline.Name), name) {
		return InterfaceCompatReport{}, errors.New("baseline and current must reference the same name")
	}
	if !strings.EqualFold(strings.TrimSpace(baseline.Kind), kind) {
		return InterfaceCompatReport{}, errors.New("baseline and current kinds must match")
	}

	breaking := make([]InterfaceChange, 0)
	hints := make([]string, 0)

	for key, prevType := range baseline.Inputs {
		nextType, ok := current.Inputs[key]
		if !ok {
			breaking = append(breaking, InterfaceChange{
				Field:       "input:" + key,
				BeforeType:  strings.TrimSpace(prevType),
				Description: "required input removed",
			})
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(prevType), strings.TrimSpace(nextType)) {
			breaking = append(breaking, InterfaceChange{
				Field:       "input:" + key,
				BeforeType:  strings.TrimSpace(prevType),
				AfterType:   strings.TrimSpace(nextType),
				Description: "input type changed",
			})
		}
	}
	for key := range current.Inputs {
		if _, ok := baseline.Inputs[key]; !ok {
			hints = append(hints, "new input added: "+key)
		}
	}

	for key, prevType := range baseline.Outputs {
		nextType, ok := current.Outputs[key]
		if !ok {
			breaking = append(breaking, InterfaceChange{
				Field:       "output:" + key,
				BeforeType:  strings.TrimSpace(prevType),
				Description: "output removed",
			})
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(prevType), strings.TrimSpace(nextType)) {
			breaking = append(breaking, InterfaceChange{
				Field:       "output:" + key,
				BeforeType:  strings.TrimSpace(prevType),
				AfterType:   strings.TrimSpace(nextType),
				Description: "output type changed",
			})
		}
	}
	for key := range current.Outputs {
		if _, ok := baseline.Outputs[key]; !ok {
			hints = append(hints, "new output added: "+key)
		}
	}

	sort.Slice(breaking, func(i, j int) bool { return breaking[i].Field < breaking[j].Field })
	sort.Strings(hints)
	return InterfaceCompatReport{
		Kind:             kind,
		Name:             name,
		FromVersion:      strings.TrimSpace(baseline.Version),
		ToVersion:        strings.TrimSpace(current.Version),
		Breaking:         len(breaking) > 0,
		BreakingChanges:  breaking,
		NonBreakingHints: hints,
	}, nil
}
