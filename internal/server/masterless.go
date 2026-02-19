package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleMasterlessMode(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.masterless.Mode())
	case http.MethodPost:
		var req control.MasterlessModeInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		mode, err := s.masterless.SetMode(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "execution.masterless.mode.updated",
			Message: "masterless mode configuration updated",
			Fields: map[string]any{
				"enabled":          mode.Enabled,
				"state_root":       mode.StateRoot,
				"default_strategy": mode.DefaultStrategy,
			},
		}, true)
		writeJSON(w, http.StatusOK, mode)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMasterlessRender(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		StateTemplate    string                `json:"state_template"`
		Strategy         string                `json:"strategy,omitempty"`
		Layers           []control.PillarLayer `json:"layers,omitempty"`
		Lookups          []string              `json:"lookups,omitempty"`
		Vars             map[string]string     `json:"vars,omitempty"`
		Role             string                `json:"role,omitempty"`
		Environment      string                `json:"environment,omitempty"`
		DataBagRefs      []string              `json:"data_bag_refs,omitempty"`
		DataBagPasswords map[string]string     `json:"data_bag_passwords,omitempty"`
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
	layers := append([]control.PillarLayer{}, req.Layers...)
	if strings.TrimSpace(req.Role) != "" {
		role, err := s.roleEnv.GetRole(req.Role)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		layers = append(layers,
			control.PillarLayer{Name: "role/" + role.Name + "/default", Data: role.DefaultAttributes},
			control.PillarLayer{Name: "role/" + role.Name + "/override", Data: role.OverrideAttributes},
		)
	}
	if strings.TrimSpace(req.Environment) != "" {
		env, err := s.roleEnv.GetEnvironment(req.Environment)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		layers = append(layers,
			control.PillarLayer{Name: "environment/" + env.Name + "/default", Data: env.DefaultAttributes},
			control.PillarLayer{Name: "environment/" + env.Name + "/override", Data: env.OverrideAttributes},
			control.PillarLayer{Name: "environment/" + env.Name + "/policy_overrides", Data: env.PolicyOverrides},
		)
	}
	for _, ref := range req.DataBagRefs {
		ref = strings.TrimSpace(ref)
		parts := strings.SplitN(ref, "/", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "data_bag_refs must use bag/item format"})
			return
		}
		passphrase := req.DataBagPasswords[ref]
		item, err := s.dataBags.Get(parts[0], parts[1], passphrase)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		layers = append(layers, control.PillarLayer{
			Name: "data_bag/" + item.Bag + "/" + item.Item,
			Data: item.Data,
		})
	}
	result, err := s.masterless.Render(control.MasterlessRenderInput{
		StateTemplate: req.StateTemplate,
		Strategy:      req.Strategy,
		Layers:        layers,
		Lookups:       req.Lookups,
		Vars:          req.Vars,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "execution.masterless.rendered",
		Message: "masterless state render completed",
		Fields: map[string]any{
			"deterministic":   result.Deterministic,
			"missing_tokens":  result.MissingTokens,
			"resolved_layers": len(layers),
		},
	}, true)
	writeJSON(w, http.StatusOK, result)
}
