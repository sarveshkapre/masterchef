package server

import (
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/masterchef/masterchef/internal/config"
)

func (s *Server) handleInventoryGroups(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		configPath := strings.TrimSpace(r.URL.Query().Get("config_path"))
		if configPath == "" {
			configPath = filepath.Join(baseDir, "masterchef.yaml")
		} else if !filepath.IsAbs(configPath) {
			configPath = filepath.Join(baseDir, configPath)
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		byRole := map[string][]string{}
		byLabel := map[string][]string{}
		byTopology := map[string][]string{}
		for _, host := range cfg.Inventory.Hosts {
			hostName := strings.TrimSpace(host.Name)
			for _, role := range host.Roles {
				role = strings.TrimSpace(strings.ToLower(role))
				if role == "" {
					continue
				}
				byRole[role] = appendUniqueString(byRole[role], hostName)
			}
			for k, v := range host.Labels {
				key := strings.TrimSpace(strings.ToLower(k))
				val := strings.TrimSpace(strings.ToLower(v))
				if key == "" || val == "" {
					continue
				}
				byLabel[key+"="+val] = appendUniqueString(byLabel[key+"="+val], hostName)
			}
			for k, v := range host.Topology {
				key := strings.TrimSpace(strings.ToLower(k))
				val := strings.TrimSpace(strings.ToLower(v))
				if key == "" || val == "" {
					continue
				}
				byTopology[key+"="+val] = appendUniqueString(byTopology[key+"="+val], hostName)
			}
		}
		sortGroupMap(byRole)
		sortGroupMap(byLabel)
		sortGroupMap(byTopology)

		writeJSON(w, http.StatusOK, map[string]any{
			"config_path": configPath,
			"host_count":  len(cfg.Inventory.Hosts),
			"by_role":     byRole,
			"by_label":    byLabel,
			"by_topology": byTopology,
		})
	}
}

func appendUniqueString(in []string, value string) []string {
	for _, item := range in {
		if item == value {
			return in
		}
	}
	return append(in, value)
}

func sortGroupMap(m map[string][]string) {
	for k := range m {
		sort.Strings(m[k])
	}
}
