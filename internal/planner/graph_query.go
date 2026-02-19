package planner

import "sort"

type GraphQueryRequest struct {
	ResourceID string `json:"resource_id"`
	Direction  string `json:"direction,omitempty"` // upstream|downstream|both
	Depth      int    `json:"depth,omitempty"`     // 0 means unlimited
}

type GraphQueryResult struct {
	ResourceID string   `json:"resource_id"`
	Upstream   []string `json:"upstream,omitempty"`
	Downstream []string `json:"downstream,omitempty"`
	Impacted   []string `json:"impacted,omitempty"`
}

func QueryGraph(plan *Plan, req GraphQueryRequest) GraphQueryResult {
	if plan == nil {
		return GraphQueryResult{ResourceID: req.ResourceID}
	}
	direction := normalizeGraphDirection(req.Direction)
	up, down := buildDependencyMaps(plan)
	res := GraphQueryResult{ResourceID: req.ResourceID}
	if direction == "upstream" || direction == "both" {
		res.Upstream = traverseDependencies(up, req.ResourceID, req.Depth)
	}
	if direction == "downstream" || direction == "both" {
		res.Downstream = traverseDependencies(down, req.ResourceID, req.Depth)
	}
	merged := map[string]struct{}{}
	for _, id := range res.Upstream {
		merged[id] = struct{}{}
	}
	for _, id := range res.Downstream {
		merged[id] = struct{}{}
	}
	res.Impacted = make([]string, 0, len(merged))
	for id := range merged {
		res.Impacted = append(res.Impacted, id)
	}
	sort.Strings(res.Impacted)
	return res
}

func buildDependencyMaps(plan *Plan) (map[string][]string, map[string][]string) {
	upstream := map[string][]string{}
	downstream := map[string][]string{}
	for _, step := range plan.Steps {
		id := step.Resource.ID
		if _, ok := upstream[id]; !ok {
			upstream[id] = []string{}
		}
		if _, ok := downstream[id]; !ok {
			downstream[id] = []string{}
		}
		for _, dep := range step.Resource.DependsOn {
			upstream[id] = append(upstream[id], dep)
			downstream[dep] = append(downstream[dep], id)
		}
	}
	for key := range upstream {
		sort.Strings(upstream[key])
	}
	for key := range downstream {
		sort.Strings(downstream[key])
	}
	return upstream, downstream
}

func traverseDependencies(graph map[string][]string, root string, depth int) []string {
	if root == "" {
		return nil
	}
	type nodeDepth struct {
		id    string
		depth int
	}
	queue := []nodeDepth{{id: root, depth: 0}}
	seen := map[string]struct{}{root: {}}
	out := make([]string, 0)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if depth > 0 && cur.depth >= depth {
			continue
		}
		neighbors := graph[cur.id]
		for _, next := range neighbors {
			if _, ok := seen[next]; ok {
				continue
			}
			seen[next] = struct{}{}
			out = append(out, next)
			queue = append(queue, nodeDepth{id: next, depth: cur.depth + 1})
		}
	}
	sort.Strings(out)
	return out
}

func normalizeGraphDirection(in string) string {
	switch in {
	case "upstream", "downstream", "both":
		return in
	default:
		return "both"
	}
}
