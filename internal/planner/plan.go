package planner

import (
	"fmt"
	"sort"

	"github.com/masterchef/masterchef/internal/config"
)

type Plan struct {
	Steps []Step `json:"steps"`
}

type Step struct {
	Order    int             `json:"order"`
	Host     config.Host     `json:"host"`
	Resource config.Resource `json:"resource"`
}

// Build constructs a deterministic topological plan from config resources.
func Build(cfg *config.Config) (*Plan, error) {
	idToRes := map[string]config.Resource{}
	inDegree := map[string]int{}
	graph := map[string][]string{}

	for _, r := range cfg.Resources {
		idToRes[r.ID] = r
		inDegree[r.ID] = 0
	}
	hostByName := map[string]config.Host{}
	for _, h := range cfg.Inventory.Hosts {
		hostByName[h.Name] = h
	}

	for _, r := range cfg.Resources {
		for _, dep := range r.DependsOn {
			graph[dep] = append(graph[dep], r.ID)
			inDegree[r.ID]++
		}
	}

	queue := make([]string, 0)
	for id, d := range inDegree {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)

	ordered := make([]string, 0, len(cfg.Resources))
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		ordered = append(ordered, cur)

		children := graph[cur]
		sort.Strings(children)
		for _, c := range children {
			inDegree[c]--
			if inDegree[c] == 0 {
				queue = append(queue, c)
			}
		}
		sort.Strings(queue)
	}

	if len(ordered) != len(cfg.Resources) {
		return nil, fmt.Errorf("cycle detected in resource dependency graph")
	}

	steps := make([]Step, 0, len(ordered))
	for i, id := range ordered {
		steps = append(steps, Step{
			Order:    i + 1,
			Host:     hostByName[idToRes[id].Host],
			Resource: idToRes[id],
		})
	}
	return &Plan{Steps: steps}, nil
}
