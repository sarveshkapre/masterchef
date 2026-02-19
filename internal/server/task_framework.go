package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleTaskDefinitions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.tasks.ListTasks())
	case http.MethodPost:
		var req control.TaskDefinitionInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.tasks.RegisterTask(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "tasks.definition.created",
			Message: "task definition registered",
			Fields: map[string]any{
				"task_id":     item.ID,
				"module":      item.Module,
				"action":      item.Action,
				"primitive":   item.Primitive,
				"param_count": len(item.Parameters),
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTaskDefinitionByID(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/tasks/definitions/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "tasks" || parts[2] != "definitions" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.tasks.GetTask(parts[3])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task definition not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleTaskPlans(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.tasks.ListPlans())
	case http.MethodPost:
		var req control.TaskPlanInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.tasks.RegisterPlan(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "tasks.plan.created",
			Message: "task plan registered",
			Fields: map[string]any{
				"plan_id":    item.ID,
				"step_count": len(item.Steps),
				"plan_name":  item.Name,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTaskPlanAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/tasks/plans/{id}
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "tasks" || parts[2] != "plans" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	switch {
	case len(parts) == 4 && r.Method == http.MethodGet:
		item, ok := s.tasks.GetPlan(parts[3])
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "task plan not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	case len(parts) == 5 && parts[4] == "preview" && r.Method == http.MethodPost:
		var req control.TaskPlanPreviewInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		preview, err := s.tasks.PreviewPlan(parts[3], req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, preview)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
