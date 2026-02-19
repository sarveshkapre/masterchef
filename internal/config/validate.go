package config

import (
	"fmt"
	"sort"
	"strings"
)

func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if strings.TrimSpace(cfg.Version) == "" {
		return fmt.Errorf("version is required")
	}
	strategy := strings.ToLower(strings.TrimSpace(cfg.Execution.Strategy))
	switch strategy {
	case "", "linear", "free", "serial":
	default:
		return fmt.Errorf("execution.strategy must be one of linear, free, serial")
	}
	if cfg.Execution.Serial < 0 {
		return fmt.Errorf("execution.serial must be >= 0")
	}
	if cfg.Execution.MaxFailPercentage < 0 || cfg.Execution.MaxFailPercentage > 100 {
		return fmt.Errorf("execution.max_fail_percentage must be between 0 and 100")
	}
	cfg.Execution.FailureDomain = strings.ToLower(strings.TrimSpace(cfg.Execution.FailureDomain))
	switch cfg.Execution.FailureDomain {
	case "", "rack", "zone", "region":
	default:
		return fmt.Errorf("execution.failure_domain must be one of rack, zone, region")
	}

	hostSet := map[string]struct{}{}
	for i, h := range cfg.Inventory.Hosts {
		if strings.TrimSpace(h.Name) == "" {
			return fmt.Errorf("inventory.hosts[%d].name is required", i)
		}
		if _, ok := hostSet[h.Name]; ok {
			return fmt.Errorf("duplicate host name %q", h.Name)
		}
		hostSet[h.Name] = struct{}{}

		if h.Transport == "" {
			cfg.Inventory.Hosts[i].Transport = "local"
		}
		cfg.Inventory.Hosts[i].Transport = strings.ToLower(strings.TrimSpace(cfg.Inventory.Hosts[i].Transport))
		switch cfg.Inventory.Hosts[i].Transport {
		case "local", "ssh", "winrm", "auto":
		default:
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.Inventory.Hosts[i].Transport)), "plugin/") {
				break
			}
			return fmt.Errorf("host %q has unsupported transport %q", h.Name, h.Transport)
		}
		cfg.Inventory.Hosts[i].Address = strings.TrimSpace(cfg.Inventory.Hosts[i].Address)
		cfg.Inventory.Hosts[i].User = strings.TrimSpace(cfg.Inventory.Hosts[i].User)
		cfg.Inventory.Hosts[i].JumpAddress = strings.TrimSpace(cfg.Inventory.Hosts[i].JumpAddress)
		cfg.Inventory.Hosts[i].JumpUser = strings.TrimSpace(cfg.Inventory.Hosts[i].JumpUser)
		cfg.Inventory.Hosts[i].ProxyCommand = strings.TrimSpace(cfg.Inventory.Hosts[i].ProxyCommand)
		if cfg.Inventory.Hosts[i].Port < 0 || cfg.Inventory.Hosts[i].Port > 65535 {
			return fmt.Errorf("host %q has invalid port %d", h.Name, cfg.Inventory.Hosts[i].Port)
		}
		if cfg.Inventory.Hosts[i].JumpPort < 0 || cfg.Inventory.Hosts[i].JumpPort > 65535 {
			return fmt.Errorf("host %q has invalid jump_port %d", h.Name, cfg.Inventory.Hosts[i].JumpPort)
		}
		if len(cfg.Inventory.Hosts[i].Capabilities) > 0 {
			seen := map[string]struct{}{}
			caps := make([]string, 0, len(cfg.Inventory.Hosts[i].Capabilities))
			for _, cap := range cfg.Inventory.Hosts[i].Capabilities {
				cap = strings.ToLower(strings.TrimSpace(cap))
				if cap == "" {
					continue
				}
				switch cap {
				case "local", "ssh", "winrm":
				default:
					return fmt.Errorf("host %q has unsupported capability %q", h.Name, cap)
				}
				if _, ok := seen[cap]; ok {
					continue
				}
				seen[cap] = struct{}{}
				caps = append(caps, cap)
			}
			sort.Strings(caps)
			cfg.Inventory.Hosts[i].Capabilities = caps
		}
		if len(cfg.Inventory.Hosts[i].Roles) > 0 {
			seen := map[string]struct{}{}
			roles := make([]string, 0, len(cfg.Inventory.Hosts[i].Roles))
			for _, role := range cfg.Inventory.Hosts[i].Roles {
				role = strings.ToLower(strings.TrimSpace(role))
				if role == "" {
					continue
				}
				if _, ok := seen[role]; ok {
					continue
				}
				seen[role] = struct{}{}
				roles = append(roles, role)
			}
			sort.Strings(roles)
			cfg.Inventory.Hosts[i].Roles = roles
		}
		if len(cfg.Inventory.Hosts[i].Labels) > 0 {
			labels := map[string]string{}
			for k, v := range cfg.Inventory.Hosts[i].Labels {
				key := strings.ToLower(strings.TrimSpace(k))
				if key == "" {
					continue
				}
				labels[key] = strings.TrimSpace(v)
			}
			cfg.Inventory.Hosts[i].Labels = labels
		}
		if len(cfg.Inventory.Hosts[i].Topology) > 0 {
			topology := map[string]string{}
			for k, v := range cfg.Inventory.Hosts[i].Topology {
				key := strings.ToLower(strings.TrimSpace(k))
				if key == "" {
					continue
				}
				topology[key] = strings.TrimSpace(v)
			}
			cfg.Inventory.Hosts[i].Topology = topology
		}
	}

	resSet := map[string]struct{}{}
	for i := range cfg.Resources {
		r := &cfg.Resources[i]
		if strings.TrimSpace(r.ID) == "" {
			return fmt.Errorf("resources[%d].id is required", i)
		}
		if _, ok := resSet[r.ID]; ok {
			return fmt.Errorf("duplicate resource id %q", r.ID)
		}
		resSet[r.ID] = struct{}{}

		if _, ok := hostSet[r.Host]; !ok {
			return fmt.Errorf("resource %q references unknown host %q", r.ID, r.Host)
		}
		if strings.TrimSpace(r.DelegateTo) != "" {
			if _, ok := hostSet[r.DelegateTo]; !ok {
				return fmt.Errorf("resource %q delegate_to references unknown host %q", r.ID, r.DelegateTo)
			}
		}
		r.BecomeUser = strings.TrimSpace(r.BecomeUser)
		if r.BecomeUser != "" {
			r.Become = true
		}
		switch r.Type {
		case "file":
			if r.Become {
				return fmt.Errorf("resource %q privilege escalation is only supported for command resources", r.ID)
			}
			if strings.TrimSpace(r.RescueCommand) != "" || strings.TrimSpace(r.AlwaysCommand) != "" {
				return fmt.Errorf("resource %q block/rescue/always hooks are only supported for command resources", r.ID)
			}
			if strings.TrimSpace(r.Path) == "" {
				return fmt.Errorf("resource %q file.path is required", r.ID)
			}
		case "command":
			if strings.TrimSpace(r.Command) == "" {
				return fmt.Errorf("resource %q command.command is required", r.ID)
			}
			r.RescueCommand = strings.TrimSpace(r.RescueCommand)
			r.AlwaysCommand = strings.TrimSpace(r.AlwaysCommand)
			if r.Retries < 0 {
				return fmt.Errorf("resource %q command.retries must be >= 0", r.ID)
			}
			if r.RetryDelaySeconds < 0 {
				return fmt.Errorf("resource %q command.retry_delay_seconds must be >= 0", r.ID)
			}
		default:
			return fmt.Errorf("resource %q has unsupported type %q", r.ID, r.Type)
		}
		if len(r.Tags) > 0 {
			seenTags := map[string]struct{}{}
			clean := make([]string, 0, len(r.Tags))
			for _, tag := range r.Tags {
				tag = strings.ToLower(strings.TrimSpace(tag))
				if tag == "" {
					continue
				}
				if _, ok := seenTags[tag]; ok {
					continue
				}
				seenTags[tag] = struct{}{}
				clean = append(clean, tag)
			}
			sort.Strings(clean)
			r.Tags = clean
		}
	}

	for _, r := range cfg.Resources {
		for _, dep := range r.DependsOn {
			if _, ok := resSet[dep]; !ok {
				return fmt.Errorf("resource %q depends on unknown resource %q", r.ID, dep)
			}
		}
		for _, dep := range r.Require {
			if _, ok := resSet[dep]; !ok {
				return fmt.Errorf("resource %q require references unknown resource %q", r.ID, dep)
			}
		}
		for _, dep := range r.Subscribe {
			if _, ok := resSet[dep]; !ok {
				return fmt.Errorf("resource %q subscribe references unknown resource %q", r.ID, dep)
			}
		}
		for _, target := range r.Before {
			if _, ok := resSet[target]; !ok {
				return fmt.Errorf("resource %q before references unknown resource %q", r.ID, target)
			}
		}
		for _, target := range r.Notify {
			if _, ok := resSet[target]; !ok {
				return fmt.Errorf("resource %q notify references unknown resource %q", r.ID, target)
			}
		}
	}
	return nil
}
