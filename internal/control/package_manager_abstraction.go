package control

import (
	"errors"
	"sort"
	"strings"
)

type PackageManagerBackend struct {
	ID        string   `json:"id"`
	Platforms []string `json:"platforms"`
	Distros   []string `json:"distros,omitempty"`
	Supports  []string `json:"supports"`
}

type PackageManagerResolveInput struct {
	OS             string `json:"os"`
	Distro         string `json:"distro,omitempty"`
	Preferred      string `json:"preferred,omitempty"`
	RequiredAction string `json:"required_action,omitempty"`
}

type PackageManagerResolveResult struct {
	Compatible  bool                    `json:"compatible"`
	Selected    string                  `json:"selected,omitempty"`
	Candidates  []PackageManagerBackend `json:"candidates,omitempty"`
	Reason      string                  `json:"reason"`
	RequiredFor string                  `json:"required_for,omitempty"`
}

type PackageManagerActionInput struct {
	OS        string `json:"os"`
	Distro    string `json:"distro,omitempty"`
	Preferred string `json:"preferred,omitempty"`
	Action    string `json:"action"` // install|upgrade|remove|hold|unhold
	Package   string `json:"package"`
	Version   string `json:"version,omitempty"`
}

type PackageManagerActionPlan struct {
	Allowed       bool     `json:"allowed"`
	Manager       string   `json:"manager,omitempty"`
	Action        string   `json:"action"`
	Package       string   `json:"package"`
	Command       []string `json:"command,omitempty"`
	Reason        string   `json:"reason"`
	BlockedReason string   `json:"blocked_reason,omitempty"`
}

type PackageManagerAbstractionStore struct {
	backends map[string]PackageManagerBackend
}

func NewPackageManagerAbstractionStore() *PackageManagerAbstractionStore {
	backends := []PackageManagerBackend{
		{ID: "apt", Platforms: []string{"linux"}, Distros: []string{"debian", "ubuntu"}, Supports: []string{"install", "upgrade", "remove", "hold", "unhold"}},
		{ID: "dnf", Platforms: []string{"linux"}, Distros: []string{"almalinux", "amazonlinux", "centos", "fedora", "rhel", "rocky"}, Supports: []string{"install", "upgrade", "remove", "hold", "unhold"}},
		{ID: "yum", Platforms: []string{"linux"}, Distros: []string{"centos", "rhel"}, Supports: []string{"install", "upgrade", "remove"}},
		{ID: "zypper", Platforms: []string{"linux"}, Distros: []string{"opensuse", "sles", "suse"}, Supports: []string{"install", "upgrade", "remove", "hold", "unhold"}},
		{ID: "brew", Platforms: []string{"macos"}, Supports: []string{"install", "upgrade", "remove"}},
		{ID: "winget", Platforms: []string{"windows"}, Supports: []string{"install", "upgrade", "remove"}},
		{ID: "chocolatey", Platforms: []string{"windows"}, Supports: []string{"install", "upgrade", "remove"}},
	}
	store := &PackageManagerAbstractionStore{backends: map[string]PackageManagerBackend{}}
	for _, backend := range backends {
		b := backend
		b.Platforms = normalizeStringList(b.Platforms)
		b.Distros = normalizeStringList(b.Distros)
		b.Supports = normalizeStringList(b.Supports)
		store.backends[b.ID] = b
	}
	return store
}

func (s *PackageManagerAbstractionStore) List() []PackageManagerBackend {
	out := make([]PackageManagerBackend, 0, len(s.backends))
	for _, backend := range s.backends {
		out = append(out, backend)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *PackageManagerAbstractionStore) Resolve(in PackageManagerResolveInput) PackageManagerResolveResult {
	osName := normalizePlatform(in.OS)
	distro := normalizePlatform(in.Distro)
	preferred := strings.ToLower(strings.TrimSpace(in.Preferred))
	requiredAction := strings.ToLower(strings.TrimSpace(in.RequiredAction))
	candidates := s.candidatesFor(osName, distro)
	if len(candidates) == 0 {
		return PackageManagerResolveResult{
			Compatible: false,
			Reason:     "no package manager backend matches os/distro",
		}
	}

	selected := pickPackageManager(candidates, preferred, osName, distro)
	if selected.ID == "" {
		return PackageManagerResolveResult{
			Compatible: false,
			Candidates: candidates,
			Reason:     "no compatible package manager backend selected",
		}
	}
	if requiredAction != "" && !packageManagerSupports(selected, requiredAction) {
		return PackageManagerResolveResult{
			Compatible:  false,
			Selected:    selected.ID,
			Candidates:  candidates,
			RequiredFor: requiredAction,
			Reason:      "selected package manager does not support required action",
		}
	}
	return PackageManagerResolveResult{
		Compatible: true,
		Selected:   selected.ID,
		Candidates: candidates,
		Reason:     "resolved package manager backend for target platform",
	}
}

func (s *PackageManagerAbstractionStore) RenderAction(in PackageManagerActionInput) (PackageManagerActionPlan, error) {
	action := strings.ToLower(strings.TrimSpace(in.Action))
	pkg := strings.TrimSpace(in.Package)
	if action == "" || pkg == "" {
		return PackageManagerActionPlan{}, errors.New("action and package are required")
	}
	resolved := s.Resolve(PackageManagerResolveInput{
		OS:             in.OS,
		Distro:         in.Distro,
		Preferred:      in.Preferred,
		RequiredAction: action,
	})
	if !resolved.Compatible {
		return PackageManagerActionPlan{
			Allowed:       false,
			Action:        action,
			Package:       pkg,
			Reason:        "failed to resolve package manager backend",
			BlockedReason: resolved.Reason,
		}, nil
	}
	command, err := renderPackageManagerCommand(resolved.Selected, action, pkg, strings.TrimSpace(in.Version))
	if err != nil {
		return PackageManagerActionPlan{}, err
	}
	return PackageManagerActionPlan{
		Allowed: true,
		Manager: resolved.Selected,
		Action:  action,
		Package: pkg,
		Command: command,
		Reason:  "rendered package action via platform-specific package manager abstraction",
	}, nil
}

func (s *PackageManagerAbstractionStore) candidatesFor(osName, distro string) []PackageManagerBackend {
	out := make([]PackageManagerBackend, 0)
	for _, backend := range s.backends {
		if !containsPackageManagerStringFold(backend.Platforms, osName) {
			continue
		}
		if len(backend.Distros) > 0 && distro != "" && !containsPackageManagerStringFold(backend.Distros, distro) {
			continue
		}
		out = append(out, backend)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func pickPackageManager(candidates []PackageManagerBackend, preferred, osName, distro string) PackageManagerBackend {
	if preferred != "" {
		for _, item := range candidates {
			if item.ID == preferred {
				return item
			}
		}
	}
	switch osName {
	case "windows":
		for _, id := range []string{"winget", "chocolatey"} {
			for _, item := range candidates {
				if item.ID == id {
					return item
				}
			}
		}
	case "macos":
		for _, item := range candidates {
			if item.ID == "brew" {
				return item
			}
		}
	case "linux":
		if containsPackageManagerStringFold([]string{"ubuntu", "debian"}, distro) {
			for _, item := range candidates {
				if item.ID == "apt" {
					return item
				}
			}
		}
		if containsPackageManagerStringFold([]string{"sles", "suse", "opensuse"}, distro) {
			for _, item := range candidates {
				if item.ID == "zypper" {
					return item
				}
			}
		}
		for _, id := range []string{"dnf", "yum", "apt", "zypper"} {
			for _, item := range candidates {
				if item.ID == id {
					return item
				}
			}
		}
	}
	if len(candidates) == 0 {
		return PackageManagerBackend{}
	}
	return candidates[0]
}

func packageManagerSupports(backend PackageManagerBackend, action string) bool {
	return containsPackageManagerStringFold(backend.Supports, action)
}

func renderPackageManagerCommand(manager, action, pkg, version string) ([]string, error) {
	switch manager {
	case "apt":
		switch action {
		case "install":
			if version != "" {
				return []string{"apt-get", "install", "-y", pkg + "=" + version}, nil
			}
			return []string{"apt-get", "install", "-y", pkg}, nil
		case "upgrade":
			return []string{"apt-get", "install", "--only-upgrade", "-y", pkg}, nil
		case "remove":
			return []string{"apt-get", "remove", "-y", pkg}, nil
		case "hold":
			return []string{"apt-mark", "hold", pkg}, nil
		case "unhold":
			return []string{"apt-mark", "unhold", pkg}, nil
		}
	case "dnf", "yum":
		bin := manager
		switch action {
		case "install":
			return []string{bin, "install", "-y", pkg}, nil
		case "upgrade":
			return []string{bin, "upgrade", "-y", pkg}, nil
		case "remove":
			return []string{bin, "remove", "-y", pkg}, nil
		case "hold":
			return []string{bin, "versionlock", "add", pkg}, nil
		case "unhold":
			return []string{bin, "versionlock", "delete", pkg}, nil
		}
	case "zypper":
		switch action {
		case "install":
			return []string{"zypper", "--non-interactive", "install", pkg}, nil
		case "upgrade":
			return []string{"zypper", "--non-interactive", "update", pkg}, nil
		case "remove":
			return []string{"zypper", "--non-interactive", "remove", pkg}, nil
		case "hold":
			return []string{"zypper", "--non-interactive", "addlock", pkg}, nil
		case "unhold":
			return []string{"zypper", "--non-interactive", "removelock", pkg}, nil
		}
	case "brew":
		switch action {
		case "install":
			return []string{"brew", "install", pkg}, nil
		case "upgrade":
			return []string{"brew", "upgrade", pkg}, nil
		case "remove":
			return []string{"brew", "uninstall", pkg}, nil
		}
	case "winget":
		switch action {
		case "install":
			return []string{"winget", "install", "--id", pkg, "--silent"}, nil
		case "upgrade":
			return []string{"winget", "upgrade", "--id", pkg, "--silent"}, nil
		case "remove":
			return []string{"winget", "uninstall", "--id", pkg, "--silent"}, nil
		}
	case "chocolatey":
		switch action {
		case "install":
			return []string{"choco", "install", pkg, "-y"}, nil
		case "upgrade":
			return []string{"choco", "upgrade", pkg, "-y"}, nil
		case "remove":
			return []string{"choco", "uninstall", pkg, "-y"}, nil
		}
	}
	return nil, errors.New("unsupported package manager action")
}

func normalizePlatform(in string) string {
	v := strings.ToLower(strings.TrimSpace(in))
	switch v {
	case "darwin", "mac", "osx":
		return "macos"
	default:
		return v
	}
}

func containsPackageManagerStringFold(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == target {
			return true
		}
	}
	return false
}
