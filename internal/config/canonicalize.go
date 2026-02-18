package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func Canonicalize(cfg *Config) Config {
	if cfg == nil {
		return Config{}
	}
	out := *cfg
	out.Inventory.Hosts = append([]Host{}, cfg.Inventory.Hosts...)
	sort.Slice(out.Inventory.Hosts, func(i, j int) bool {
		return out.Inventory.Hosts[i].Name < out.Inventory.Hosts[j].Name
	})
	for i := range out.Inventory.Hosts {
		out.Inventory.Hosts[i].Roles = append([]string{}, out.Inventory.Hosts[i].Roles...)
		sort.Strings(out.Inventory.Hosts[i].Roles)
	}

	out.Resources = append([]Resource{}, cfg.Resources...)
	for i := range out.Resources {
		out.Resources[i].DependsOn = append([]string{}, out.Resources[i].DependsOn...)
		sort.Strings(out.Resources[i].DependsOn)
		out.Resources[i].Tags = append([]string{}, out.Resources[i].Tags...)
		sort.Strings(out.Resources[i].Tags)
	}
	sort.Slice(out.Resources, func(i, j int) bool {
		return out.Resources[i].ID < out.Resources[j].ID
	})
	return out
}

func MarshalCanonical(cfg *Config, format string) ([]byte, error) {
	canon := Canonicalize(cfg)
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "yaml"
	}
	switch format {
	case "json":
		return json.MarshalIndent(canon, "", "  ")
	case "yaml", "yml":
		return yaml.Marshal(canon)
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}
