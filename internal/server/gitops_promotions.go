package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleGitOpsPromotions(w http.ResponseWriter, r *http.Request) {
	type createReq struct {
		Name           string   `json:"name"`
		Stages         []string `json:"stages,omitempty"`
		ArtifactDigest string   `json:"artifact_digest"`
		Actor          string   `json:"actor,omitempty"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.gitopsPromotions.List())
	case http.MethodPost:
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.gitopsPromotions.Create(control.GitOpsPromotionInput{
			Name:           req.Name,
			Stages:         req.Stages,
			ArtifactDigest: req.ArtifactDigest,
			Actor:          req.Actor,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "gitops.promotion.created",
			Message: "promotion pipeline created with immutable artifact pin",
			Fields: map[string]any{
				"promotion_id":    item.ID,
				"name":            item.Name,
				"artifact_digest": item.ArtifactDigest,
				"current_stage":   item.CurrentStage,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGitOpsPromotionAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/gitops/promotions/{id} or /{id}/advance
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "gitops" || parts[2] != "promotions" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := strings.TrimSpace(parts[3])
	if id == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if len(parts) == 4 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, ok := s.gitopsPromotions.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "promotion pipeline not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) != 5 || parts[4] != "advance" || r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	type advanceReq struct {
		ArtifactDigest string `json:"artifact_digest"`
		Actor          string `json:"actor,omitempty"`
		Note           string `json:"note,omitempty"`
	}
	var req advanceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	item, err := s.gitopsPromotions.Advance(id, req.ArtifactDigest, req.Actor, req.Note)
	if err != nil {
		code := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "mismatch") || strings.Contains(strings.ToLower(err.Error()), "completed") {
			code = http.StatusConflict
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "gitops.promotion.advanced",
		Message: "promotion advanced to next stage",
		Fields: map[string]any{
			"promotion_id":    item.ID,
			"artifact_digest": item.ArtifactDigest,
			"current_stage":   item.CurrentStage,
			"status":          item.Status,
		},
	}, true)
	writeJSON(w, http.StatusOK, item)
}
