package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleBulkPreview(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Name       string                  `json:"name"`
		Operations []control.BulkOperation `json:"operations"`
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req reqBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if len(req.Operations) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "operations are required"})
		return
	}

	previews := make([]control.BulkOperationPreview, 0, len(req.Operations))
	conflicts := detectBulkConflicts(req.Operations)
	for _, op := range req.Operations {
		norm := normalizeBulkOperation(op)
		reason := s.validateBulkOperation(norm)
		previews = append(previews, control.BulkOperationPreview{
			Operation: norm,
			Ready:     reason == "",
			Reason:    reason,
		})
	}
	if s.queue.EmergencyStatus().Active {
		conflicts = append(conflicts, "emergency stop active; launch/execute operations should be deferred")
	}
	preview := s.bulk.SavePreview(req.Name, previews, conflicts)
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleBulkExecute(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		PreviewToken string `json:"preview_token"`
		Confirm      bool   `json:"confirm"`
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req reqBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "confirm=true is required for staged execution"})
		return
	}

	preview, err := s.bulk.ConsumePreview(req.PreviewToken)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if !preview.Ready {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":     "preview has unresolved conflicts",
			"preview":   preview,
			"conflicts": preview.Conflicts,
		})
		return
	}

	results := make([]control.BulkExecutionResult, 0, len(preview.Operations))
	applied := 0
	failed := 0
	for _, op := range preview.Operations {
		if !op.Ready {
			failed++
			results = append(results, control.BulkExecutionResult{
				Operation: op.Operation,
				Applied:   false,
				Error:     op.Reason,
			})
			continue
		}
		err := s.applyBulkOperation(op.Operation)
		item := control.BulkExecutionResult{
			Operation: op.Operation,
			Applied:   err == nil,
		}
		if err != nil {
			item.Error = err.Error()
			failed++
		} else {
			applied++
		}
		results = append(results, item)
	}
	status := http.StatusOK
	if failed > 0 {
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, map[string]any{
		"preview_token": req.PreviewToken,
		"applied_count": applied,
		"failed_count":  failed,
		"results":       results,
	})
}

func normalizeBulkOperation(op control.BulkOperation) control.BulkOperation {
	op.Action = strings.TrimSpace(strings.ToLower(op.Action))
	op.TargetType = strings.TrimSpace(strings.ToLower(op.TargetType))
	op.TargetID = strings.TrimSpace(op.TargetID)
	if op.Params == nil {
		op.Params = map[string]any{}
	}
	return op
}

func detectBulkConflicts(ops []control.BulkOperation) []string {
	conflicts := make([]string, 0)
	perTarget := map[string]map[string]int{}
	for _, raw := range ops {
		op := normalizeBulkOperation(raw)
		key := op.TargetType + ":" + op.TargetID
		if perTarget[key] == nil {
			perTarget[key] = map[string]int{}
		}
		perTarget[key][op.Action]++
		if perTarget[key][op.Action] > 1 {
			conflicts = append(conflicts, "duplicate operation for "+key+" action="+op.Action)
		}
	}
	for key, actions := range perTarget {
		if actions["schedule.enable"] > 0 && actions["schedule.disable"] > 0 {
			conflicts = append(conflicts, "conflicting schedule enable/disable operations for "+key)
		}
		if actions["view.pin"] > 0 && actions["view.unpin"] > 0 {
			conflicts = append(conflicts, "conflicting view pin/unpin operations for "+key)
		}
		if actions["runbook.approve"] > 0 && actions["runbook.deprecate"] > 0 {
			conflicts = append(conflicts, "conflicting runbook approve/deprecate operations for "+key)
		}
	}
	return conflicts
}

func (s *Server) validateBulkOperation(op control.BulkOperation) string {
	if op.Action == "" || op.TargetType == "" || op.TargetID == "" {
		return "action, target_type, and target_id are required"
	}
	switch op.Action {
	case "schedule.enable", "schedule.disable":
		if op.TargetType != "schedule" {
			return "schedule actions require target_type=schedule"
		}
		for _, sched := range s.scheduler.List() {
			if sched.ID == op.TargetID {
				return ""
			}
		}
		return "schedule not found"
	case "runbook.approve", "runbook.deprecate":
		if op.TargetType != "runbook" {
			return "runbook actions require target_type=runbook"
		}
		if _, err := s.runbooks.Get(op.TargetID); err != nil {
			return err.Error()
		}
		return ""
	case "view.pin", "view.unpin":
		if op.TargetType != "view" {
			return "view actions require target_type=view"
		}
		if _, err := s.views.Get(op.TargetID); err != nil {
			return err.Error()
		}
		return ""
	case "template.delete":
		if op.TargetType != "template" {
			return "template.delete requires target_type=template"
		}
		if _, ok := s.templates.Get(op.TargetID); !ok {
			return "template not found"
		}
		return ""
	default:
		return "unsupported bulk action"
	}
}

func (s *Server) applyBulkOperation(op control.BulkOperation) error {
	switch op.Action {
	case "schedule.enable":
		if ok := s.scheduler.Enable(op.TargetID); !ok {
			return errors.New("schedule not found")
		}
		return nil
	case "schedule.disable":
		if ok := s.scheduler.Disable(op.TargetID); !ok {
			return errors.New("schedule not found")
		}
		return nil
	case "runbook.approve":
		_, err := s.runbooks.Approve(op.TargetID)
		return err
	case "runbook.deprecate":
		_, err := s.runbooks.Deprecate(op.TargetID)
		return err
	case "view.pin":
		_, err := s.views.SetPinned(op.TargetID, true)
		return err
	case "view.unpin":
		_, err := s.views.SetPinned(op.TargetID, false)
		return err
	case "template.delete":
		return s.templates.Delete(op.TargetID)
	default:
		return errors.New("unsupported bulk action")
	}
}
