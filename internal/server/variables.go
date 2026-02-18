package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

var errInvalidDataBagRef = errors.New("include_data_bags entries must use bag/item format")

type variableResolveRequest struct {
	Layers             []control.VariableLayer `json:"layers"`
	HardFail           bool                    `json:"hard_fail"`
	IncludeRole        string                  `json:"include_role,omitempty"`
	IncludeEnvironment string                  `json:"include_environment,omitempty"`
	IncludeDataBags    []string                `json:"include_data_bags,omitempty"` // bag/item
	DataBagPassphrases map[string]string       `json:"data_bag_passphrases,omitempty"`
}

func (s *Server) handleVariableResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req variableResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	layers, err := s.expandVariableLayers(req)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
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
		"result": result,
	})
}

func (s *Server) handleVariableExplain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req variableResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	layers, err := s.expandVariableLayers(req)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
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
		"merged":       result.Merged,
		"precedence":   result.Precedence,
		"conflicts":    result.Conflicts,
		"warnings":     result.Warnings,
		"source_graph": result.SourceGraph,
		"generated_at": result.GeneratedAt,
	})
}

func (s *Server) expandVariableLayers(req variableResolveRequest) ([]control.VariableLayer, error) {
	layers := append([]control.VariableLayer{}, req.Layers...)
	if roleName := strings.TrimSpace(req.IncludeRole); roleName != "" {
		role, err := s.roleEnv.GetRole(roleName)
		if err != nil {
			return nil, err
		}
		layers = append(layers,
			control.VariableLayer{Name: "role/" + role.Name + "/default_attributes", Data: role.DefaultAttributes},
			control.VariableLayer{Name: "role/" + role.Name + "/override_attributes", Data: role.OverrideAttributes},
		)
	}
	if envName := strings.TrimSpace(req.IncludeEnvironment); envName != "" {
		env, err := s.roleEnv.GetEnvironment(envName)
		if err != nil {
			return nil, err
		}
		layers = append(layers,
			control.VariableLayer{Name: "environment/" + env.Name + "/default_attributes", Data: env.DefaultAttributes},
			control.VariableLayer{Name: "environment/" + env.Name + "/override_attributes", Data: env.OverrideAttributes},
			control.VariableLayer{Name: "environment/" + env.Name + "/policy_overrides", Data: env.PolicyOverrides},
		)
	}
	for _, ref := range req.IncludeDataBags {
		ref = strings.TrimSpace(ref)
		parts := strings.SplitN(ref, "/", 2)
		if len(parts) != 2 {
			return nil, errInvalidDataBagRef
		}
		item, err := s.dataBags.Get(parts[0], parts[1], req.DataBagPassphrases[ref])
		if err != nil {
			return nil, err
		}
		layers = append(layers, control.VariableLayer{
			Name: "data_bag/" + item.Bag + "/" + item.Item,
			Data: item.Data,
		})
	}
	return layers, nil
}
