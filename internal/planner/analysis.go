package planner

import "sort"

type BlastRadius struct {
	TotalSteps     int      `json:"total_steps"`
	AffectedHosts  []string `json:"affected_hosts"`
	AffectedTypes  []string `json:"affected_types"`
	MaxOrder       int      `json:"max_order"`
	EstimatedScope string   `json:"estimated_scope"` // low, medium, high
}

func AnalyzeBlastRadius(p *Plan) BlastRadius {
	hostSet := map[string]struct{}{}
	typeSet := map[string]struct{}{}

	maxOrder := 0
	for _, s := range p.Steps {
		hostSet[s.Resource.Host] = struct{}{}
		typeSet[s.Resource.Type] = struct{}{}
		if s.Order > maxOrder {
			maxOrder = s.Order
		}
	}

	hosts := make([]string, 0, len(hostSet))
	for h := range hostSet {
		hosts = append(hosts, h)
	}
	types := make([]string, 0, len(typeSet))
	for t := range typeSet {
		types = append(types, t)
	}
	sort.Strings(hosts)
	sort.Strings(types)

	scope := "low"
	switch {
	case len(p.Steps) >= 25 || len(hosts) >= 10:
		scope = "high"
	case len(p.Steps) >= 8 || len(hosts) >= 3:
		scope = "medium"
	}

	return BlastRadius{
		TotalSteps:     len(p.Steps),
		AffectedHosts:  hosts,
		AffectedTypes:  types,
		MaxOrder:       maxOrder,
		EstimatedScope: scope,
	}
}
