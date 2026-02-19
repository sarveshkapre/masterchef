package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleNodeClassificationRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.nodeClassification.List())
	case http.MethodPost:
		var req control.NodeClassificationRuleInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		rule, err := s.nodeClassification.Upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, rule)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleNodeClassificationRuleByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := filepath.Base(r.URL.Path)
	if id == "" || id == "classification-rules" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rule id is required"})
		return
	}
	rule, ok := s.nodeClassification.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "classification rule not found"})
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleNodeClassify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.NodeClassificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	writeJSON(w, http.StatusOK, s.nodeClassification.Evaluate(req))
}
