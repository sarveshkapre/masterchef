package control

import (
	"errors"
	"sort"
	"strings"
)

type ObjectModelEntry struct {
	Canonical   string   `json:"canonical"`
	CLI         string   `json:"cli"`
	API         string   `json:"api"`
	UI          string   `json:"ui"`
	Aliases     []string `json:"aliases,omitempty"`
	Description string   `json:"description,omitempty"`
}

type ObjectModelRegistry struct {
	entries []ObjectModelEntry
	index   map[string]ObjectModelEntry
}

func NewObjectModelRegistry() *ObjectModelRegistry {
	entries := []ObjectModelEntry{
		{Canonical: "run", CLI: "run", API: "run", UI: "Run", Aliases: []string{"job-run", "execution"}, Description: "Single policy/config execution result."},
		{Canonical: "plan", CLI: "plan", API: "plan", UI: "Plan", Aliases: []string{"preview", "dry-run-plan"}, Description: "Deterministic execution plan before apply."},
		{Canonical: "runbook", CLI: "runbook", API: "runbook", UI: "Runbook", Aliases: []string{"playbook", "procedure"}, Description: "Reusable approved operational procedure."},
		{Canonical: "workflow", CLI: "workflow", API: "workflow", UI: "Workflow", Aliases: []string{"pipeline", "orchestration-flow"}, Description: "Multi-step orchestration template or execution."},
		{Canonical: "policy_bundle", CLI: "policy bundle", API: "policy bundle", UI: "Policy Bundle", Aliases: []string{"policyfile", "catalog"}, Description: "Versioned desired-state artifact with lock metadata."},
		{Canonical: "drift", CLI: "drift", API: "drift", UI: "Drift", Aliases: []string{"config-drift", "state-drift"}, Description: "Desired vs observed mismatch state."},
		{Canonical: "host", CLI: "host", API: "host", UI: "Node", Aliases: []string{"node", "managed-host"}, Description: "Managed compute target."},
		{Canonical: "module", CLI: "module", API: "module", UI: "Module", Aliases: []string{"cookbook", "collection", "package"}, Description: "Reusable content package for tasks/policies/providers."},
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Canonical < entries[j].Canonical })
	index := map[string]ObjectModelEntry{}
	for _, entry := range entries {
		registerObjectModelAlias(index, entry.Canonical, entry)
		registerObjectModelAlias(index, entry.CLI, entry)
		registerObjectModelAlias(index, entry.API, entry)
		registerObjectModelAlias(index, entry.UI, entry)
		for _, alias := range entry.Aliases {
			registerObjectModelAlias(index, alias, entry)
		}
	}
	return &ObjectModelRegistry{
		entries: entries,
		index:   index,
	}
}

func (r *ObjectModelRegistry) List() []ObjectModelEntry {
	out := make([]ObjectModelEntry, 0, len(r.entries))
	for _, entry := range r.entries {
		copied := entry
		copied.Aliases = append([]string{}, entry.Aliases...)
		out = append(out, copied)
	}
	return out
}

func (r *ObjectModelRegistry) Resolve(term string) (ObjectModelEntry, error) {
	key := normalizeObjectModelTerm(term)
	if key == "" {
		return ObjectModelEntry{}, errors.New("term is required")
	}
	entry, ok := r.index[key]
	if !ok {
		return ObjectModelEntry{}, errors.New("term not found")
	}
	copied := entry
	copied.Aliases = append([]string{}, entry.Aliases...)
	return copied, nil
}

func registerObjectModelAlias(index map[string]ObjectModelEntry, raw string, entry ObjectModelEntry) {
	key := normalizeObjectModelTerm(raw)
	if key == "" {
		return
	}
	index[key] = entry
}

func normalizeObjectModelTerm(in string) string {
	in = strings.TrimSpace(strings.ToLower(in))
	if in == "" {
		return ""
	}
	in = strings.ReplaceAll(in, "_", "-")
	in = strings.ReplaceAll(in, " ", "-")
	return in
}
