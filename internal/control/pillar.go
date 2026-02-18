package control

import (
	"encoding/json"
	"errors"
	"strings"
)

type PillarLayer struct {
	Name string         `json:"name"`
	Data map[string]any `json:"data"`
}

type PillarResolveRequest struct {
	Strategy string        `json:"strategy"` // merge-first|merge-last|overwrite|remove
	Layers   []PillarLayer `json:"layers"`
	Lookup   string        `json:"lookup,omitempty"`
	Default  any           `json:"default,omitempty"`
}

type PillarResolveResult struct {
	Strategy string         `json:"strategy"`
	Layers   []string       `json:"layers"`
	Merged   map[string]any `json:"merged"`
	Found    bool           `json:"found"`
	Value    any            `json:"value,omitempty"`
}

func ResolvePillar(req PillarResolveRequest) (PillarResolveResult, error) {
	strategy := normalizePillarStrategy(req.Strategy)
	if strategy == "" {
		return PillarResolveResult{}, errors.New("strategy must be one of merge-first, merge-last, overwrite, remove")
	}
	merged := map[string]any{}
	layerNames := make([]string, 0, len(req.Layers))
	for _, layer := range req.Layers {
		layerNames = append(layerNames, strings.TrimSpace(layer.Name))
		data := clonePillarMap(layer.Data)
		applyPillarStrategy(merged, data, strategy)
	}

	result := PillarResolveResult{
		Strategy: strategy,
		Layers:   layerNames,
		Merged:   merged,
		Found:    true,
		Value:    merged,
	}
	lookup := strings.TrimSpace(req.Lookup)
	if lookup != "" {
		value, ok := lookupPillarPath(merged, lookup)
		if !ok {
			result.Found = false
			result.Value = req.Default
			return result, nil
		}
		result.Value = value
	}
	return result, nil
}

func normalizePillarStrategy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "merge-first":
		return "merge-first"
	case "merge-last":
		return "merge-last"
	case "overwrite":
		return "overwrite"
	case "remove":
		return "remove"
	default:
		return ""
	}
}

func applyPillarStrategy(dst, src map[string]any, strategy string) {
	for k, v := range src {
		switch strategy {
		case "merge-first":
			existing, exists := dst[k]
			if !exists {
				dst[k] = clonePillarAny(v)
				continue
			}
			existingMap, okExisting := existing.(map[string]any)
			srcMap, okSrc := v.(map[string]any)
			if okExisting && okSrc {
				applyPillarStrategy(existingMap, srcMap, strategy)
				dst[k] = existingMap
			}
		case "merge-last":
			existing, exists := dst[k]
			existingMap, okExisting := existing.(map[string]any)
			srcMap, okSrc := v.(map[string]any)
			if exists && okExisting && okSrc {
				applyPillarStrategy(existingMap, srcMap, strategy)
				dst[k] = existingMap
				continue
			}
			dst[k] = clonePillarAny(v)
		case "overwrite":
			dst[k] = clonePillarAny(v)
		case "remove":
			if v == nil {
				delete(dst, k)
				continue
			}
			existing, exists := dst[k]
			existingMap, okExisting := existing.(map[string]any)
			srcMap, okSrc := v.(map[string]any)
			if exists && okExisting && okSrc {
				applyPillarStrategy(existingMap, srcMap, strategy)
				dst[k] = existingMap
				continue
			}
			dst[k] = clonePillarAny(v)
		}
	}
}

func lookupPillarPath(data map[string]any, path string) (any, bool) {
	cur := any(data)
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		nextMap, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := nextMap[part]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func clonePillarMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	buf, _ := json.Marshal(in)
	var out map[string]any
	_ = json.Unmarshal(buf, &out)
	if out == nil {
		return map[string]any{}
	}
	return out
}

func clonePillarAny(in any) any {
	buf, _ := json.Marshal(in)
	var out any
	_ = json.Unmarshal(buf, &out)
	return out
}
