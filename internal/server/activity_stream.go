package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleActivityStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}

	replayLimit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("replay_limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			replayLimit = n
		}
	}
	var since time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			since = parsed
		}
	}
	var until time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("until")); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			until = parsed
		}
	}
	typePrefix := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type_prefix")))
	contains := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("contains")))

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	if replayLimit > 0 {
		replay := s.events.Query(control.EventQuery{
			Since:      since,
			Until:      until,
			TypePrefix: typePrefix,
			Contains:   contains,
			Limit:      replayLimit,
			Desc:       false,
		})
		for _, event := range replay {
			if err := writeActivitySSE(w, event); err != nil {
				return
			}
			flusher.Flush()
		}
	}

	subID, eventsCh := s.events.Subscribe(256)
	defer s.events.Unsubscribe(subID)

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case event, ok := <-eventsCh:
			if !ok {
				return
			}
			if !activityEventMatches(event, since, until, typePrefix, contains) {
				continue
			}
			if err := writeActivitySSE(w, event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeActivitySSE(w io.Writer, event control.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, "event: activity\n"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "data: "+string(payload)+"\n\n"); err != nil {
		return err
	}
	return nil
}

func activityEventMatches(event control.Event, since, until time.Time, typePrefix, contains string) bool {
	if !since.IsZero() && event.Time.Before(since) {
		return false
	}
	if !until.IsZero() && event.Time.After(until) {
		return false
	}
	if typePrefix != "" && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(event.Type)), typePrefix) {
		return false
	}
	if contains != "" {
		msg := strings.ToLower(event.Message)
		typ := strings.ToLower(event.Type)
		if !strings.Contains(msg, contains) && !strings.Contains(typ, contains) {
			return false
		}
	}
	return true
}
