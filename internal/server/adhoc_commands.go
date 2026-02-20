package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleAdHocPolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.adhocCommands.Policy())
	case http.MethodPost:
		var req control.AdHocGuardrailPolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.adhocCommands.SetPolicy(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAdHocCommands(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Command        string `json:"command"`
		Reason         string `json:"reason,omitempty"`
		RequestedBy    string `json:"requested_by,omitempty"`
		Host           string `json:"host,omitempty"`
		TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
		DryRun         bool   `json:"dry_run,omitempty"`
	}
	switch r.Method {
	case http.MethodGet:
		limit := 100
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		writeJSON(w, http.StatusOK, s.adhocCommands.List(limit))
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		normalized, allowed, reasons, err := s.adhocCommands.Evaluate(control.AdHocCommandRequest{
			Command:        req.Command,
			Reason:         req.Reason,
			RequestedBy:    req.RequestedBy,
			Host:           req.Host,
			TimeoutSeconds: req.TimeoutSeconds,
			DryRun:         req.DryRun,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		result := control.AdHocCommandResult{
			Command:        normalized.Command,
			Reason:         normalized.Reason,
			RequestedBy:    normalized.RequestedBy,
			Host:           normalized.Host,
			DryRun:         normalized.DryRun,
			Allowed:        allowed,
			BlockedReasons: append([]string{}, reasons...),
			Status:         "approved",
		}
		if !allowed {
			result.Status = "blocked"
			recorded := s.adhocCommands.Record(result)
			s.recordEvent(control.Event{
				Type:    "command.adhoc.blocked",
				Message: "ad-hoc command blocked by guardrails",
				Fields: map[string]any{
					"command_id":      recorded.ID,
					"requested_by":    recorded.RequestedBy,
					"host":            recorded.Host,
					"blocked_reasons": recorded.BlockedReasons,
				},
			}, true)
			writeJSON(w, http.StatusConflict, recorded)
			return
		}

		start := time.Now().UTC()
		if normalized.DryRun {
			result.Status = "approved"
			result.Output = "dry-run approved; execution skipped"
		} else {
			execRes := executeAdHocCommand(r.Context(), normalized.Command, normalized.TimeoutSeconds)
			result.Status = execRes.Status
			result.ExitCode = execRes.ExitCode
			result.Output = execRes.Output
			result.DurationMillis = execRes.DurationMillis
		}
		if result.DurationMillis == 0 {
			result.DurationMillis = time.Since(start).Milliseconds()
		}

		recorded := s.adhocCommands.Record(result)
		eventType := "command.adhoc.executed"
		if recorded.Status == "failed" {
			eventType = "command.adhoc.failed"
		}
		s.recordEvent(control.Event{
			Type:    eventType,
			Message: "ad-hoc command processed",
			Fields: map[string]any{
				"command_id":   recorded.ID,
				"requested_by": recorded.RequestedBy,
				"host":         recorded.Host,
				"dry_run":      recorded.DryRun,
				"status":       recorded.Status,
				"exit_code":    recorded.ExitCode,
			},
		}, true)
		writeJSON(w, http.StatusOK, recorded)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type adHocExecutionResult struct {
	Status         string
	ExitCode       int
	Output         string
	DurationMillis int64
}

func executeAdHocCommand(parent context.Context, command string, timeoutSeconds int) adHocExecutionResult {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-lc", command)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	start := time.Now().UTC()
	err := cmd.Run()
	duration := time.Since(start).Milliseconds()
	result := adHocExecutionResult{
		Status:         "succeeded",
		ExitCode:       0,
		Output:         strings.TrimSpace(out.String()),
		DurationMillis: duration,
	}
	if ctx.Err() == context.DeadlineExceeded {
		result.Status = "failed"
		result.ExitCode = 124
		result.Output = trimAdHocOutput(result.Output + "\ncommand timed out")
		return result
	}
	if err != nil {
		result.Status = "failed"
		var exitErr *exec.ExitError
		if ok := errors.As(err, &exitErr); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
		}
		result.Output = trimAdHocOutput(result.Output)
		return result
	}
	result.Output = trimAdHocOutput(result.Output)
	return result
}

func trimAdHocOutput(output string) string {
	const maxLen = 4096
	output = strings.TrimSpace(output)
	if len(output) <= maxLen {
		return output
	}
	return output[:maxLen] + "\n...truncated..."
}
