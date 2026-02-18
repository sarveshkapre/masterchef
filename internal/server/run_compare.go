package server

import (
	"net/http"
	"sort"
	"strings"

	"github.com/masterchef/masterchef/internal/state"
)

type runResourceSnapshot struct {
	ResourceID string `json:"resource_id"`
	Type       string `json:"type"`
	Host       string `json:"host"`
	Changed    bool   `json:"changed"`
	Skipped    bool   `json:"skipped"`
	Message    string `json:"message,omitempty"`
}

type runDiff struct {
	ResourceID string               `json:"resource_id"`
	Type       string               `json:"type"`
	Host       string               `json:"host"`
	From       *runResourceSnapshot `json:"from,omitempty"`
	To         *runResourceSnapshot `json:"to,omitempty"`
	Difference string               `json:"difference"`
}

func (s *Server) handleRunCompare(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		runA := strings.TrimSpace(r.URL.Query().Get("run_a"))
		runB := strings.TrimSpace(r.URL.Query().Get("run_b"))
		runs, err := state.New(baseDir).ListRuns(2000)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if runA == "" || runB == "" {
			failed, success, ok := pickFailedAndSuccessfulRuns(runs)
			if !ok {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_a/run_b required when there are no comparable failed+successful runs"})
				return
			}
			if runA == "" {
				runA = failed.ID
			}
			if runB == "" {
				runB = success.ID
			}
		}

		recA, err := state.New(baseDir).GetRun(runA)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "run_a not found"})
			return
		}
		recB, err := state.New(baseDir).GetRun(runB)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "run_b not found"})
			return
		}

		diffs := compareRuns(recA, recB)
		writeJSON(w, http.StatusOK, map[string]any{
			"run_a": map[string]any{
				"id":     recA.ID,
				"status": recA.Status,
			},
			"run_b": map[string]any{
				"id":     recB.ID,
				"status": recB.Status,
			},
			"diff_count": len(diffs),
			"diffs":      diffs,
		})
	}
}

func pickFailedAndSuccessfulRuns(runs []state.RunRecord) (state.RunRecord, state.RunRecord, bool) {
	var failed state.RunRecord
	var success state.RunRecord
	hasFailed := false
	hasSuccess := false
	for _, run := range runs {
		if !hasFailed && run.Status == state.RunFailed {
			failed = run
			hasFailed = true
		}
		if !hasSuccess && run.Status == state.RunSucceeded {
			success = run
			hasSuccess = true
		}
		if hasFailed && hasSuccess {
			return failed, success, true
		}
	}
	return state.RunRecord{}, state.RunRecord{}, false
}

func compareRuns(a, b state.RunRecord) []runDiff {
	from := map[string]runResourceSnapshot{}
	to := map[string]runResourceSnapshot{}
	keys := map[string]struct{}{}

	keyFor := func(res state.ResourceRun) string {
		return strings.TrimSpace(res.Host) + "|" + strings.TrimSpace(res.ResourceID) + "|" + strings.TrimSpace(res.Type)
	}
	for _, res := range a.Results {
		k := keyFor(res)
		keys[k] = struct{}{}
		from[k] = runResourceSnapshot{
			ResourceID: res.ResourceID,
			Type:       res.Type,
			Host:       res.Host,
			Changed:    res.Changed,
			Skipped:    res.Skipped,
			Message:    res.Message,
		}
	}
	for _, res := range b.Results {
		k := keyFor(res)
		keys[k] = struct{}{}
		to[k] = runResourceSnapshot{
			ResourceID: res.ResourceID,
			Type:       res.Type,
			Host:       res.Host,
			Changed:    res.Changed,
			Skipped:    res.Skipped,
			Message:    res.Message,
		}
	}

	diffs := make([]runDiff, 0)
	for k := range keys {
		f, hasFrom := from[k]
		t, hasTo := to[k]
		switch {
		case hasFrom && !hasTo:
			fc := f
			diffs = append(diffs, runDiff{
				ResourceID: f.ResourceID,
				Type:       f.Type,
				Host:       f.Host,
				From:       &fc,
				Difference: "resource present only in run_a",
			})
		case !hasFrom && hasTo:
			tc := t
			diffs = append(diffs, runDiff{
				ResourceID: t.ResourceID,
				Type:       t.Type,
				Host:       t.Host,
				To:         &tc,
				Difference: "resource present only in run_b",
			})
		default:
			diff := compareSnapshot(f, t)
			if diff == "" {
				continue
			}
			fc := f
			tc := t
			diffs = append(diffs, runDiff{
				ResourceID: f.ResourceID,
				Type:       f.Type,
				Host:       f.Host,
				From:       &fc,
				To:         &tc,
				Difference: diff,
			})
		}
	}
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].Host != diffs[j].Host {
			return diffs[i].Host < diffs[j].Host
		}
		if diffs[i].ResourceID != diffs[j].ResourceID {
			return diffs[i].ResourceID < diffs[j].ResourceID
		}
		return diffs[i].Type < diffs[j].Type
	})
	return diffs
}

func compareSnapshot(from, to runResourceSnapshot) string {
	parts := make([]string, 0, 3)
	if from.Changed != to.Changed {
		parts = append(parts, "changed flag differs")
	}
	if from.Skipped != to.Skipped {
		parts = append(parts, "skipped flag differs")
	}
	if strings.TrimSpace(from.Message) != strings.TrimSpace(to.Message) {
		parts = append(parts, "message differs")
	}
	return strings.Join(parts, "; ")
}
