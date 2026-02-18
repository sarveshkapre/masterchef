package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handlePillarResolve(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Strategy           string                `json:"strategy"`
		Layers             []control.PillarLayer `json:"layers"`
		Lookup             string                `json:"lookup"`
		Default            any                   `json:"default"`
		Role               string                `json:"role"`
		Environment        string                `json:"environment"`
		DataBagRefs        []string              `json:"data_bag_refs"`
		DataBagPassphrases map[string]string     `json:"data_bag_passphrases"`
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
		passphrase := req.DataBagPassphrases[ref]
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

	result, err := control.ResolvePillar(control.PillarResolveRequest{
		Strategy: req.Strategy,
		Layers:   layers,
		Lookup:   req.Lookup,
		Default:  req.Default,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"result": result,
		"input": map[string]any{
			"strategy":        req.Strategy,
			"lookup":          req.Lookup,
			"role":            req.Role,
			"environment":     req.Environment,
			"data_bag_refs":   req.DataBagRefs,
			"resolved_layers": len(layers),
		},
	})
}
