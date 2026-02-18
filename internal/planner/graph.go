package planner

import (
	"fmt"
	"sort"
	"strings"
)

func ToDOT(p *Plan) string {
	var b strings.Builder
	b.WriteString("digraph masterchef_plan {\n")
	b.WriteString("  rankdir=LR;\n")

	ids := make([]string, 0, len(p.Steps))
	stepByID := map[string]Step{}
	for _, s := range p.Steps {
		ids = append(ids, s.Resource.ID)
		stepByID[s.Resource.ID] = s
	}
	sort.Strings(ids)

	for _, id := range ids {
		s := stepByID[id]
		label := fmt.Sprintf("%s\\n(type=%s host=%s order=%d)", id, s.Resource.Type, s.Resource.Host, s.Order)
		b.WriteString(fmt.Sprintf("  \"%s\" [label=\"%s\"];\n", id, label))
	}

	for _, id := range ids {
		s := stepByID[id]
		deps := append([]string{}, s.Resource.DependsOn...)
		sort.Strings(deps)
		for _, dep := range deps {
			b.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\";\n", dep, s.Resource.ID))
		}
	}

	b.WriteString("}\n")
	return b.String()
}
