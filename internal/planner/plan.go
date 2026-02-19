package planner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/masterchef/masterchef/internal/config"
)

type Plan struct {
	Execution config.Execution `json:"execution,omitempty"`
	Steps     []Step           `json:"steps"`
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
		h = resolveHostTransport(h)
		hostByName[h.Name] = h
	}

	edgeSet := map[string]struct{}{}
	addEdge := func(from, to string) {
		if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
			return
		}
		key := from + "->" + to
		if _, exists := edgeSet[key]; exists {
			return
		}
		edgeSet[key] = struct{}{}
		graph[from] = append(graph[from], to)
		inDegree[to]++
	}
	for _, r := range cfg.Resources {
		deps := append([]string{}, r.DependsOn...)
		deps = append(deps, r.Require...)
		deps = append(deps, r.Subscribe...)
		for _, dep := range deps {
			addEdge(dep, r.ID)
		}
		targets := append([]string{}, r.Before...)
		targets = append(targets, r.Notify...)
		for _, target := range targets {
			addEdge(r.ID, target)
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
		execHost := idToRes[id].Host
		if strings.TrimSpace(idToRes[id].DelegateTo) != "" {
			execHost = idToRes[id].DelegateTo
		}
		steps = append(steps, Step{
			Order:    i + 1,
			Host:     hostByName[execHost],
			Resource: idToRes[id],
		})
	}
	return &Plan{
		Execution: cfg.Execution,
		Steps:     steps,
	}, nil
}

func resolveHostTransport(h config.Host) config.Host {
	transport := strings.ToLower(strings.TrimSpace(h.Transport))
	if transport == "" {
		transport = "local"
	}
	if transport != "auto" {
		h.Transport = transport
		return h
	}
	h.Transport = discoverHostTransport(h)
	return h
}

func discoverHostTransport(h config.Host) string {
	capabilities := map[string]struct{}{}
	for _, cap := range h.Capabilities {
		cap = strings.ToLower(strings.TrimSpace(cap))
		if cap == "" {
			continue
		}
		capabilities[cap] = struct{}{}
	}
	if _, ok := capabilities["local"]; ok && isLocalEndpoint(h) {
		return "local"
	}
	if _, ok := capabilities["winrm"]; ok {
		return "winrm"
	}
	if _, ok := capabilities["ssh"]; ok {
		return "ssh"
	}

	if isLocalEndpoint(h) {
		return "local"
	}
	if h.Port == 5985 || h.Port == 5986 {
		return "winrm"
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(h.Labels["os"])), "windows") {
		return "winrm"
	}
	if strings.TrimSpace(h.User) != "" || strings.TrimSpace(h.JumpAddress) != "" || strings.TrimSpace(h.ProxyCommand) != "" {
		return "ssh"
	}
	return "ssh"
}

func isLocalEndpoint(h config.Host) bool {
	target := strings.ToLower(strings.TrimSpace(h.Address))
	if target == "" {
		target = strings.ToLower(strings.TrimSpace(h.Name))
	}
	switch target {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
