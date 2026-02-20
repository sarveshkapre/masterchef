package control

import (
	"context"
	"errors"
	"strings"
	"time"
)

type PolicyInputResolveRequest struct {
	Sources  []VariableSourceSpec `json:"sources"`
	Strategy string               `json:"strategy,omitempty"` // merge-first|merge-last|overwrite|remove
	Lookup   string               `json:"lookup,omitempty"`
	Default  any                  `json:"default,omitempty"`
	HardFail bool                 `json:"hard_fail,omitempty"`
}

type PolicyInputResolveResult struct {
	Strategy     string               `json:"strategy"`
	Layers       []string             `json:"layers"`
	Merged       map[string]any       `json:"merged"`
	Found        bool                 `json:"found"`
	Value        any                  `json:"value,omitempty"`
	Conflicts    []VariableConflict   `json:"conflicts,omitempty"`
	Warnings     []string             `json:"warnings,omitempty"`
	SourceGraph  []VariableSourceEdge `json:"source_graph,omitempty"`
	ResolvedFrom int                  `json:"resolved_from"`
	ResolvedAt   time.Time            `json:"resolved_at"`
}

func ResolvePolicyInputs(ctx context.Context, registry *VariableSourceRegistry, req PolicyInputResolveRequest) (PolicyInputResolveResult, error) {
	if registry == nil {
		return PolicyInputResolveResult{}, errors.New("variable source registry is required")
	}
	if len(req.Sources) == 0 {
		return PolicyInputResolveResult{}, errors.New("sources are required")
	}
	strategy := normalizePillarStrategy(req.Strategy)
	if strategy == "" {
		strategy = "merge-last"
	}

	layers, err := registry.ResolveLayers(ctx, req.Sources)
	if err != nil {
		return PolicyInputResolveResult{}, err
	}
	resolveDiag, _ := ResolveVariables(VariableResolveRequest{
		Layers:   layers,
		HardFail: false,
	})

	pillarLayers := make([]PillarLayer, 0, len(layers))
	for _, layer := range layers {
		pillarLayers = append(pillarLayers, PillarLayer{
			Name: layer.Name,
			Data: layer.Data,
		})
	}
	pillarResult, err := ResolvePillar(PillarResolveRequest{
		Strategy: strategy,
		Layers:   pillarLayers,
		Lookup:   strings.TrimSpace(req.Lookup),
		Default:  req.Default,
	})
	if err != nil {
		return PolicyInputResolveResult{}, err
	}

	result := PolicyInputResolveResult{
		Strategy:     pillarResult.Strategy,
		Layers:       append([]string{}, pillarResult.Layers...),
		Merged:       pillarResult.Merged,
		Found:        pillarResult.Found,
		Value:        pillarResult.Value,
		Conflicts:    append([]VariableConflict{}, resolveDiag.Conflicts...),
		Warnings:     append([]string{}, resolveDiag.Warnings...),
		SourceGraph:  append([]VariableSourceEdge{}, resolveDiag.SourceGraph...),
		ResolvedFrom: len(layers),
		ResolvedAt:   time.Now().UTC(),
	}
	if req.HardFail && len(result.Conflicts) > 0 {
		return result, errors.New("policy input precedence conflict detected")
	}
	return result, nil
}
