package server

import (
	"sort"
	"time"

	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/state"
)

type timelineItem struct {
	Time    time.Time      `json:"time"`
	Phase   string         `json:"phase"` // before|during|after
	Source  string         `json:"source"`
	Type    string         `json:"type"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

func (s *Server) buildRunTimeline(run state.RunRecord, beforeWindow, afterWindow time.Duration, limit int) []timelineItem {
	if limit <= 0 {
		limit = 1000
	}
	started := run.StartedAt
	ended := run.EndedAt
	if started.IsZero() {
		started = ended
	}
	if ended.IsZero() {
		ended = started
	}

	windowStart := started.Add(-beforeWindow)
	windowEnd := ended.Add(afterWindow)

	items := make([]timelineItem, 0, limit+len(run.Results)+2)
	events := s.events.List()
	for _, evt := range events {
		if !windowStart.IsZero() && evt.Time.Before(windowStart) {
			continue
		}
		if !windowEnd.IsZero() && evt.Time.After(windowEnd) {
			continue
		}
		items = append(items, timelineItem{
			Time:    evt.Time,
			Phase:   timelinePhase(evt.Time, started, ended),
			Source:  "event",
			Type:    evt.Type,
			Message: evt.Message,
			Fields:  evt.Fields,
		})
	}

	if !run.StartedAt.IsZero() {
		items = append(items, timelineItem{
			Time:    run.StartedAt,
			Phase:   "during",
			Source:  "run",
			Type:    "run.started",
			Message: "run started",
			Fields: map[string]any{
				"run_id": run.ID,
			},
		})
	}

	if len(run.Results) > 0 {
		resourceTime := started
		step := time.Second
		if !started.IsZero() && !ended.IsZero() && ended.After(started) {
			step = ended.Sub(started) / time.Duration(len(run.Results)+1)
			if step < time.Millisecond {
				step = time.Millisecond
			}
		}
		for _, res := range run.Results {
			resourceTime = resourceTime.Add(step)
			status := "unchanged"
			if res.Changed {
				status = "changed"
			}
			if res.Skipped {
				status = "skipped"
			}
			items = append(items, timelineItem{
				Time:    resourceTime,
				Phase:   timelinePhase(resourceTime, started, ended),
				Source:  "resource",
				Type:    "resource." + status,
				Message: res.ResourceID + " (" + res.Type + ") on " + res.Host,
				Fields: map[string]any{
					"resource_id": res.ResourceID,
					"resource":    res.Type,
					"host":        res.Host,
					"status":      status,
				},
			})
		}
	}

	if !run.EndedAt.IsZero() {
		items = append(items, timelineItem{
			Time:    run.EndedAt,
			Phase:   "during",
			Source:  "run",
			Type:    "run.finished",
			Message: "run finished with status " + string(run.Status),
			Fields: map[string]any{
				"run_id": run.ID,
				"status": run.Status,
			},
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if !items[i].Time.Equal(items[j].Time) {
			return items[i].Time.Before(items[j].Time)
		}
		if items[i].Source != items[j].Source {
			return items[i].Source < items[j].Source
		}
		return items[i].Type < items[j].Type
	})
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items
}

func timelinePhase(ts, started, ended time.Time) string {
	if !started.IsZero() && ts.Before(started) {
		return "before"
	}
	if !ended.IsZero() && ts.After(ended) {
		return "after"
	}
	return "during"
}

func timelineSummary(items []timelineItem) map[string]int {
	out := map[string]int{
		"before": 0,
		"during": 0,
		"after":  0,
	}
	for _, item := range items {
		out[item.Phase]++
	}
	return out
}

func timelineWindow(run state.RunRecord, beforeWindow, afterWindow time.Duration) (time.Time, time.Time) {
	started := run.StartedAt
	ended := run.EndedAt
	if started.IsZero() {
		started = ended
	}
	if ended.IsZero() {
		ended = started
	}
	if started.IsZero() && ended.IsZero() {
		return time.Time{}, time.Time{}
	}
	return started.Add(-beforeWindow), ended.Add(afterWindow)
}

func filterCorrelatedEvents(events []control.Event, windowStart, windowEnd time.Time, limit int) []control.Event {
	if limit <= 0 {
		limit = 1000
	}
	out := make([]control.Event, 0, minInt(limit, len(events)))
	for _, evt := range events {
		if !windowStart.IsZero() && evt.Time.Before(windowStart) {
			continue
		}
		if !windowEnd.IsZero() && evt.Time.After(windowEnd) {
			continue
		}
		out = append(out, evt)
		if len(out) >= limit {
			break
		}
	}
	return out
}
