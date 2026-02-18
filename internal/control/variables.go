package control

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type VariableLayer struct {
	Name string         `json:"name"`
	Data map[string]any `json:"data"`
}

type VariableResolveRequest struct {
	Layers   []VariableLayer `json:"layers"`
	HardFail bool            `json:"hard_fail"`
}

type VariableConflict struct {
	Path          string `json:"path"`
	PreviousLayer string `json:"previous_layer"`
	CurrentLayer  string `json:"current_layer"`
	PreviousValue any    `json:"previous_value"`
	CurrentValue  any    `json:"current_value"`
	Resolution    string `json:"resolution"`
	Hint          string `json:"hint"`
}

type VariableSourceEdge struct {
	Path   string `json:"path"`
	From   string `json:"from"`
	To     string `json:"to"`
	Action string `json:"action"` // set|override
}

type VariableResolveResult struct {
	Merged      map[string]any       `json:"merged"`
	Precedence  []string             `json:"precedence"`
	Conflicts   []VariableConflict   `json:"conflicts"`
	Warnings    []string             `json:"warnings"`
	SourceGraph []VariableSourceEdge `json:"source_graph"`
	GeneratedAt time.Time            `json:"generated_at"`
}

func ResolveVariables(req VariableResolveRequest) (VariableResolveResult, error) {
	merged := map[string]any{}
	conflicts := make([]VariableConflict, 0)
	warnings := make([]string, 0)
	graph := make([]VariableSourceEdge, 0)
	precedence := make([]string, 0, len(req.Layers))
	lastSourceByPath := map[string]string{}
	lastValueByPath := map[string]any{}
	overrideCountByPath := map[string]int{}

	for i, layer := range req.Layers {
		name := strings.TrimSpace(layer.Name)
		if name == "" {
			name = "layer-" + itoa(int64(i+1))
		}
		precedence = append(precedence, name)
		layerData := cloneVariableMap(layer.Data)
		mergeVariableLayer(mergeContext{
			layerName:           name,
			dest:                merged,
			source:              layerData,
			pathPrefix:          "",
			conflicts:           &conflicts,
			graph:               &graph,
			lastSourceByPath:    lastSourceByPath,
			lastValueByPath:     lastValueByPath,
			overrideCountByPath: overrideCountByPath,
		})
	}

	for path, count := range overrideCountByPath {
		if count >= 2 {
			prev := lastSourceByPath[path]
			warnings = append(warnings, "ambiguous override for "+path+" across multiple layers (latest: "+prev+"); prefer narrowing scope or renaming variable")
		}
	}

	result := VariableResolveResult{
		Merged:      merged,
		Precedence:  precedence,
		Conflicts:   conflicts,
		Warnings:    warnings,
		SourceGraph: graph,
		GeneratedAt: time.Now().UTC(),
	}
	if req.HardFail && len(conflicts) > 0 {
		return result, errors.New("variable precedence conflict detected")
	}
	return result, nil
}

type mergeContext struct {
	layerName           string
	dest                map[string]any
	source              map[string]any
	pathPrefix          string
	conflicts           *[]VariableConflict
	graph               *[]VariableSourceEdge
	lastSourceByPath    map[string]string
	lastValueByPath     map[string]any
	overrideCountByPath map[string]int
}

func mergeVariableLayer(ctx mergeContext) {
	for key, rawValue := range ctx.source {
		path := key
		if ctx.pathPrefix != "" {
			path = ctx.pathPrefix + "." + key
		}
		existing, exists := ctx.dest[key]
		incomingMap, incomingIsMap := rawValue.(map[string]any)
		existingMap, existingIsMap := existing.(map[string]any)
		if exists && incomingIsMap && existingIsMap {
			subCtx := ctx
			subCtx.dest = existingMap
			subCtx.source = incomingMap
			subCtx.pathPrefix = path
			mergeVariableLayer(subCtx)
			ctx.dest[key] = existingMap
			continue
		}

		if !exists {
			ctx.dest[key] = cloneVariableAny(rawValue)
			ctx.lastSourceByPath[path] = ctx.layerName
			ctx.lastValueByPath[path] = cloneVariableAny(rawValue)
			*ctx.graph = append(*ctx.graph, VariableSourceEdge{
				Path:   path,
				From:   "",
				To:     ctx.layerName,
				Action: "set",
			})
			continue
		}

		prevLayer := ctx.lastSourceByPath[path]
		prevVal := ctx.lastValueByPath[path]
		nextVal := cloneVariableAny(rawValue)
		if !variableValueEqual(prevVal, nextVal) {
			*ctx.conflicts = append(*ctx.conflicts, VariableConflict{
				Path:          path,
				PreviousLayer: prevLayer,
				CurrentLayer:  ctx.layerName,
				PreviousValue: cloneVariableAny(prevVal),
				CurrentValue:  cloneVariableAny(nextVal),
				Resolution:    "current layer overrides previous value",
				Hint:          "review variable precedence or split environment-specific keys to avoid accidental overrides",
			})
			ctx.overrideCountByPath[path]++
		}
		ctx.dest[key] = nextVal
		ctx.lastSourceByPath[path] = ctx.layerName
		ctx.lastValueByPath[path] = nextVal
		*ctx.graph = append(*ctx.graph, VariableSourceEdge{
			Path:   path,
			From:   prevLayer,
			To:     ctx.layerName,
			Action: "override",
		})
	}
}

func cloneVariableMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	buf, _ := json.Marshal(in)
	var out map[string]any
	_ = json.Unmarshal(buf, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func cloneVariableAny(in any) any {
	buf, _ := json.Marshal(in)
	var out any
	_ = json.Unmarshal(buf, &out)
	return out
}

func variableValueEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}
