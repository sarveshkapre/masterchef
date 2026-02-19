package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleImageBakePipelines(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.imageBaking.List())
	case http.MethodPost:
		var req control.ImageBakePipelineInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.imageBaking.Create(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		preview, _ := s.imageBaking.Plan(control.ImageBakePlanInput{PipelineID: item.ID})
		writeJSON(w, http.StatusCreated, map[string]any{
			"pipeline":     item,
			"plan_preview": preview,
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleImageBakePipelineAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/execution/image-baking/pipelines/{id}[/plan]
	if len(parts) < 5 || parts[0] != "v1" || parts[1] != "execution" || parts[2] != "image-baking" || parts[3] != "pipelines" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := parts[4]

	if len(parts) == 5 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, ok := s.imageBaking.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "image bake pipeline not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}

	if len(parts) == 6 && parts[5] == "plan" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req control.ImageBakePlanInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		req.PipelineID = id
		plan, err := s.imageBaking.Plan(req)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		if !plan.Allowed {
			writeJSON(w, http.StatusConflict, plan)
			return
		}
		writeJSON(w, http.StatusOK, plan)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}
