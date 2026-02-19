package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func Load(path string) (*Config, error) {
	resolved, err := filepath.Abs(path)
	if err != nil {
		resolved = path
	}
	cfg, err := loadComposedConfig(resolved, map[string]bool{})
	if err != nil {
		return nil, err
	}
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func loadComposedConfig(path string, stack map[string]bool) (*Config, error) {
	resolved, err := filepath.Abs(path)
	if err != nil {
		resolved = path
	}
	if stack[resolved] {
		return nil, fmt.Errorf("config composition cycle detected at %s", resolved)
	}
	stack[resolved] = true
	defer delete(stack, resolved)

	raw, err := parseConfigFile(resolved)
	if err != nil {
		return nil, err
	}
	merged := &Config{}
	baseDir := filepath.Dir(resolved)

	for _, include := range append([]string{}, raw.Includes...) {
		child, err := loadComposedConfig(resolveConfigRef(baseDir, include), stack)
		if err != nil {
			return nil, err
		}
		mergeConfig(merged, child)
	}
	for _, imp := range append([]string{}, raw.Imports...) {
		child, err := loadComposedConfig(resolveConfigRef(baseDir, imp), stack)
		if err != nil {
			return nil, err
		}
		mergeConfig(merged, child)
	}

	current := cloneConfig(raw)
	current.Includes = nil
	current.Imports = nil
	current.Overlays = nil
	mergeConfig(merged, &current)

	for _, overlay := range append([]string{}, raw.Overlays...) {
		child, err := loadComposedConfig(resolveConfigRef(baseDir, overlay), stack)
		if err != nil {
			return nil, err
		}
		mergeConfig(merged, child)
	}
	return merged, nil
}

func parseConfigFile(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		if err := json.Unmarshal(b, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse json config: %w", err)
		}
	default:
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse yaml config: %w", err)
		}
	}
	return cfg, nil
}

func resolveConfigRef(baseDir, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ref
	}
	if filepath.IsAbs(ref) {
		return ref
	}
	return filepath.Join(baseDir, ref)
}

func mergeConfig(dst *Config, src *Config) {
	if dst == nil || src == nil {
		return
	}
	if strings.TrimSpace(src.Version) != "" {
		dst.Version = src.Version
	}
	mergeInventory(&dst.Inventory, src.Inventory)
	mergeExecution(&dst.Execution, src.Execution)
	mergeResources(&dst.Resources, src.Resources)
	mergeResources(&dst.Handlers, src.Handlers)
}

func mergeInventory(dst *Inventory, src Inventory) {
	if dst == nil {
		return
	}
	index := map[string]int{}
	for i, host := range dst.Hosts {
		index[host.Name] = i
	}
	for _, host := range src.Hosts {
		if i, ok := index[host.Name]; ok {
			dst.Hosts[i] = cloneHost(host)
			continue
		}
		index[host.Name] = len(dst.Hosts)
		dst.Hosts = append(dst.Hosts, cloneHost(host))
	}
}

func mergeExecution(dst *Execution, src Execution) {
	if dst == nil {
		return
	}
	if strings.TrimSpace(src.Strategy) != "" {
		dst.Strategy = src.Strategy
	}
	if src.Serial != 0 {
		dst.Serial = src.Serial
	}
	if strings.TrimSpace(src.FailureDomain) != "" {
		dst.FailureDomain = src.FailureDomain
	}
	if src.MaxFailPercentage != 0 {
		dst.MaxFailPercentage = src.MaxFailPercentage
	}
	if src.AnyErrorsFatal {
		dst.AnyErrorsFatal = true
	}
}

func mergeResources(dst *[]Resource, src []Resource) {
	if dst == nil {
		return
	}
	index := map[string]int{}
	for i, res := range *dst {
		index[res.ID] = i
	}
	for _, res := range src {
		if i, ok := index[res.ID]; ok {
			(*dst)[i] = cloneResource(res)
			continue
		}
		index[res.ID] = len(*dst)
		*dst = append(*dst, cloneResource(res))
	}
}

func cloneConfig(in Config) Config {
	out := in
	out.Includes = append([]string{}, in.Includes...)
	out.Imports = append([]string{}, in.Imports...)
	out.Overlays = append([]string{}, in.Overlays...)
	out.Inventory = Inventory{Hosts: make([]Host, 0, len(in.Inventory.Hosts))}
	for _, h := range in.Inventory.Hosts {
		out.Inventory.Hosts = append(out.Inventory.Hosts, cloneHost(h))
	}
	out.Resources = make([]Resource, 0, len(in.Resources))
	for _, res := range in.Resources {
		out.Resources = append(out.Resources, cloneResource(res))
	}
	out.Handlers = make([]Resource, 0, len(in.Handlers))
	for _, handler := range in.Handlers {
		out.Handlers = append(out.Handlers, cloneResource(handler))
	}
	return out
}

func cloneHost(in Host) Host {
	out := in
	out.Capabilities = append([]string{}, in.Capabilities...)
	out.Roles = append([]string{}, in.Roles...)
	out.Labels = cloneStringMap(in.Labels)
	if len(in.Topology) == 0 {
		out.Topology = map[string]string{}
	} else {
		out.Topology = make(map[string]string, len(in.Topology))
		for k, v := range in.Topology {
			out.Topology[k] = v
		}
	}
	return out
}

func cloneResource(in Resource) Resource {
	out := in
	out.DependsOn = append([]string{}, in.DependsOn...)
	out.Require = append([]string{}, in.Require...)
	out.Before = append([]string{}, in.Before...)
	out.Notify = append([]string{}, in.Notify...)
	out.Subscribe = append([]string{}, in.Subscribe...)
	out.NotifyHandlers = append([]string{}, in.NotifyHandlers...)
	out.Tags = append([]string{}, in.Tags...)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
