package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleVariableSourceResolve(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Sources            []control.VariableSourceSpec `json:"sources"`
		Layers             []control.VariableLayer      `json:"layers"`
		HardFail           bool                         `json:"hard_fail"`
		IncludeRole        string                       `json:"include_role,omitempty"`
		IncludeEnvironment string                       `json:"include_environment,omitempty"`
		IncludeDataBags    []string                     `json:"include_data_bags,omitempty"`
		DataBagPassphrases map[string]string            `json:"data_bag_passphrases,omitempty"`
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

	baseLayers, err := s.expandVariableLayers(variableResolveRequest{
		Layers:             req.Layers,
		IncludeRole:        req.IncludeRole,
		IncludeEnvironment: req.IncludeEnvironment,
		IncludeDataBags:    req.IncludeDataBags,
		DataBagPassphrases: req.DataBagPassphrases,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	sourceLayers, err := s.varSources.ResolveLayers(r.Context(), req.Sources)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	layers := append(baseLayers, sourceLayers...)
	result, err := control.ResolveVariables(control.VariableResolveRequest{
		Layers:   layers,
		HardFail: req.HardFail,
	})
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":  err.Error(),
			"result": result,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"result":        result,
		"resolved_from": len(sourceLayers),
		"total_layers":  len(layers),
	})
}
