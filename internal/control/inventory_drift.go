package control

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type InventoryDriftHost struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

type InventoryDriftAnalyzeInput struct {
	Desired  []InventoryDriftHost `json:"desired"`
	Observed []InventoryDriftHost `json:"observed"`
}

type InventoryDriftEntry struct {
	Host   string `json:"host"`
	Status string `json:"status"` // missing|unexpected|label_drift
	Reason string `json:"reason"`
}

type InventoryReconcileAction struct {
	Host   string `json:"host"`
	Action string `json:"action"` // enroll|decommission|update_labels
	Reason string `json:"reason"`
}

type InventoryDriftReport struct {
	ID        string                     `json:"id"`
	CreatedAt time.Time                  `json:"created_at"`
	Summary   map[string]int             `json:"summary"`
	Entries   []InventoryDriftEntry      `json:"entries"`
	Actions   []InventoryReconcileAction `json:"actions,omitempty"`
}

type InventoryDriftStore struct {
	mu      sync.RWMutex
	nextID  int64
	reports map[string]*InventoryDriftReport
}

func NewInventoryDriftStore() *InventoryDriftStore {
	return &InventoryDriftStore{reports: map[string]*InventoryDriftReport{}}
}

func (s *InventoryDriftStore) Analyze(in InventoryDriftAnalyzeInput, withActions bool) InventoryDriftReport {
	desired := normalizeInventoryHosts(in.Desired)
	observed := normalizeInventoryHosts(in.Observed)

	entries := make([]InventoryDriftEntry, 0, len(desired)+len(observed))
	actions := make([]InventoryReconcileAction, 0, len(desired)+len(observed))
	summary := map[string]int{"missing": 0, "unexpected": 0, "label_drift": 0}

	for name, want := range desired {
		got, ok := observed[name]
		if !ok {
			entries = append(entries, InventoryDriftEntry{
				Host:   name,
				Status: "missing",
				Reason: "host exists in desired inventory but was not observed",
			})
			summary["missing"]++
			if withActions {
				actions = append(actions, InventoryReconcileAction{
					Host:   name,
					Action: "enroll",
					Reason: "re-enroll missing desired host",
				})
			}
			continue
		}
		if !labelsEqual(want.Labels, got.Labels) {
			entries = append(entries, InventoryDriftEntry{
				Host:   name,
				Status: "label_drift",
				Reason: "desired and observed labels differ",
			})
			summary["label_drift"]++
			if withActions {
				actions = append(actions, InventoryReconcileAction{
					Host:   name,
					Action: "update_labels",
					Reason: "align observed labels with desired inventory labels",
				})
			}
		}
	}
	for name := range observed {
		if _, ok := desired[name]; ok {
			continue
		}
		entries = append(entries, InventoryDriftEntry{
			Host:   name,
			Status: "unexpected",
			Reason: "host observed but not present in desired inventory",
		})
		summary["unexpected"]++
		if withActions {
			actions = append(actions, InventoryReconcileAction{
				Host:   name,
				Action: "decommission",
				Reason: "remove unexpected host from managed inventory",
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Host < entries[j].Host })
	sort.Slice(actions, func(i, j int) bool { return actions[i].Host < actions[j].Host })

	report := InventoryDriftReport{
		Summary:   summary,
		Entries:   entries,
		Actions:   actions,
		CreatedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	s.nextID++
	report.ID = "inventory-drift-report-" + itoa(s.nextID)
	s.reports[report.ID] = &report
	s.mu.Unlock()
	return report
}

func (s *InventoryDriftStore) List(limit int) []InventoryDriftReport {
	if limit <= 0 {
		limit = 50
	}
	s.mu.RLock()
	out := make([]InventoryDriftReport, 0, len(s.reports))
	for _, item := range s.reports {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func normalizeInventoryHosts(in []InventoryDriftHost) map[string]InventoryDriftHost {
	out := map[string]InventoryDriftHost{}
	for _, host := range in {
		name := strings.ToLower(strings.TrimSpace(host.Name))
		if name == "" {
			continue
		}
		labels := map[string]string{}
		for k, v := range host.Labels {
			key := strings.ToLower(strings.TrimSpace(k))
			if key == "" {
				continue
			}
			labels[key] = strings.TrimSpace(v)
		}
		out[name] = InventoryDriftHost{Name: name, Labels: labels}
	}
	return out
}

func labelsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if bv, ok := b[k]; !ok || av != bv {
			return false
		}
	}
	return true
}
