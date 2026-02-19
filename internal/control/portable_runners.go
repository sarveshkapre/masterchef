package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type PortableRunnerProfileInput struct {
	Name             string   `json:"name"`
	OSFamilies       []string `json:"os_families,omitempty"`
	Architectures    []string `json:"architectures,omitempty"`
	TransportModes   []string `json:"transport_modes,omitempty"`
	Shell            string   `json:"shell,omitempty"`
	ArtifactRef      string   `json:"artifact_ref,omitempty"`
	ChecksumSHA256   string   `json:"checksum_sha256,omitempty"`
	SupportsSudo     bool     `json:"supports_sudo"`
	SupportsRunAs    bool     `json:"supports_run_as"`
	SupportsNoPython bool     `json:"supports_no_python"`
}

type PortableRunnerProfile struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	OSFamilies       []string  `json:"os_families,omitempty"`
	Architectures    []string  `json:"architectures,omitempty"`
	TransportModes   []string  `json:"transport_modes,omitempty"`
	Shell            string    `json:"shell,omitempty"`
	ArtifactRef      string    `json:"artifact_ref,omitempty"`
	ChecksumSHA256   string    `json:"checksum_sha256,omitempty"`
	SupportsSudo     bool      `json:"supports_sudo"`
	SupportsRunAs    bool      `json:"supports_run_as"`
	SupportsNoPython bool      `json:"supports_no_python"`
	Builtin          bool      `json:"builtin"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type PortableRunnerSelectRequest struct {
	OSFamily       string `json:"os_family"`
	Architecture   string `json:"architecture"`
	TransportMode  string `json:"transport_mode,omitempty"`
	PythonRequired bool   `json:"python_required,omitempty"`
}

type PortableRunnerSelectResult struct {
	Supported bool                  `json:"supported"`
	Mode      string                `json:"mode,omitempty"`
	Reason    string                `json:"reason,omitempty"`
	Profile   PortableRunnerProfile `json:"profile,omitempty"`
}

type PortableRunnerCatalog struct {
	mu       sync.RWMutex
	nextID   int64
	profiles map[string]*PortableRunnerProfile
}

func NewPortableRunnerCatalog() *PortableRunnerCatalog {
	now := time.Now().UTC()
	profiles := map[string]*PortableRunnerProfile{}
	profiles["runner-1"] = &PortableRunnerProfile{
		ID:               "runner-1",
		Name:             "posix-static-runner",
		OSFamilies:       []string{"linux", "darwin"},
		Architectures:    []string{"amd64", "arm64"},
		TransportModes:   []string{"ssh", "local", "auto"},
		Shell:            "sh",
		ArtifactRef:      "runner://posix-static",
		ChecksumSHA256:   "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		SupportsSudo:     true,
		SupportsRunAs:    false,
		SupportsNoPython: true,
		Builtin:          true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	profiles["runner-2"] = &PortableRunnerProfile{
		ID:               "runner-2",
		Name:             "windows-powershell-runner",
		OSFamilies:       []string{"windows"},
		Architectures:    []string{"amd64", "arm64"},
		TransportModes:   []string{"winrm", "local", "auto"},
		Shell:            "powershell",
		ArtifactRef:      "runner://windows-ps1",
		ChecksumSHA256:   "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		SupportsSudo:     false,
		SupportsRunAs:    true,
		SupportsNoPython: true,
		Builtin:          true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	return &PortableRunnerCatalog{
		nextID:   2,
		profiles: profiles,
	}
}

func (c *PortableRunnerCatalog) Register(in PortableRunnerProfileInput) (PortableRunnerProfile, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return PortableRunnerProfile{}, errors.New("name is required")
	}
	osFamilies := normalizeStringSlice(in.OSFamilies)
	archs := normalizeStringSlice(in.Architectures)
	modes := normalizeStringSlice(in.TransportModes)
	if len(osFamilies) == 0 || len(archs) == 0 {
		return PortableRunnerProfile{}, errors.New("os_families and architectures are required")
	}
	now := time.Now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	item := &PortableRunnerProfile{
		ID:               "runner-" + itoa(c.nextID),
		Name:             name,
		OSFamilies:       osFamilies,
		Architectures:    archs,
		TransportModes:   modes,
		Shell:            strings.ToLower(strings.TrimSpace(in.Shell)),
		ArtifactRef:      strings.TrimSpace(in.ArtifactRef),
		ChecksumSHA256:   strings.TrimSpace(in.ChecksumSHA256),
		SupportsSudo:     in.SupportsSudo,
		SupportsRunAs:    in.SupportsRunAs,
		SupportsNoPython: in.SupportsNoPython,
		Builtin:          false,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	c.profiles[item.ID] = item
	return clonePortableRunnerProfile(*item), nil
}

func (c *PortableRunnerCatalog) List() []PortableRunnerProfile {
	c.mu.RLock()
	out := make([]PortableRunnerProfile, 0, len(c.profiles))
	for _, item := range c.profiles {
		out = append(out, clonePortableRunnerProfile(*item))
	}
	c.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Builtin != out[j].Builtin {
			return out[i].Builtin
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (c *PortableRunnerCatalog) Select(req PortableRunnerSelectRequest) (PortableRunnerSelectResult, error) {
	osFamily := strings.ToLower(strings.TrimSpace(req.OSFamily))
	arch := strings.ToLower(strings.TrimSpace(req.Architecture))
	mode := strings.ToLower(strings.TrimSpace(req.TransportMode))
	if osFamily == "" || arch == "" {
		return PortableRunnerSelectResult{}, errors.New("os_family and architecture are required")
	}
	if mode == "" {
		mode = "auto"
	}
	candidates := c.List()
	for _, candidate := range candidates {
		if !containsStringFold(candidate.OSFamilies, osFamily) {
			continue
		}
		if !containsStringFold(candidate.Architectures, arch) {
			continue
		}
		if len(candidate.TransportModes) > 0 && !containsStringFold(candidate.TransportModes, mode) {
			continue
		}
		if req.PythonRequired && candidate.SupportsNoPython {
			continue
		}
		return PortableRunnerSelectResult{
			Supported: true,
			Mode:      "pythonless-portable",
			Reason:    "portable runner selected",
			Profile:   candidate,
		}, nil
	}
	reason := "no portable runner profile matches host capabilities"
	if req.PythonRequired {
		reason = "python is required by request; no matching python-based runner profile"
	}
	return PortableRunnerSelectResult{
		Supported: false,
		Mode:      "pythonless-portable",
		Reason:    reason,
	}, nil
}

func containsStringFold(items []string, needle string) bool {
	for _, item := range items {
		if strings.EqualFold(item, needle) {
			return true
		}
	}
	return false
}

func clonePortableRunnerProfile(in PortableRunnerProfile) PortableRunnerProfile {
	out := in
	out.OSFamilies = append([]string{}, in.OSFamilies...)
	out.Architectures = append([]string{}, in.Architectures...)
	out.TransportModes = append([]string{}, in.TransportModes...)
	return out
}
