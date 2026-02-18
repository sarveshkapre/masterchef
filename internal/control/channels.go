package control

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ChannelAssignment struct {
	Component string    `json:"component"`
	Channel   string    `json:"channel"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CompatibilityResult struct {
	ControlPlaneProtocol int  `json:"control_plane_protocol"`
	AgentProtocol        int  `json:"agent_protocol"`
	Compatible           bool `json:"compatible"`
}

type SupportMatrixRow struct {
	Channel              string `json:"channel"`
	ControlPlaneProtocol int    `json:"control_plane_protocol"`
	MinAgentProtocol     int    `json:"min_agent_protocol"`
	MaxAgentProtocol     int    `json:"max_agent_protocol"`
	Policy               string `json:"policy"`
}

type SupportMatrix struct {
	GeneratedAt          time.Time          `json:"generated_at"`
	ControlPlaneProtocol int                `json:"control_plane_protocol"`
	Rows                 []SupportMatrixRow `json:"rows"`
}

type ChannelManager struct {
	mu          sync.RWMutex
	assignments map[string]*ChannelAssignment
}

func NewChannelManager() *ChannelManager {
	return &ChannelManager{assignments: map[string]*ChannelAssignment{}}
}

func (m *ChannelManager) SetChannel(component, channel string) (ChannelAssignment, error) {
	component = strings.TrimSpace(strings.ToLower(component))
	if component == "" {
		return ChannelAssignment{}, errors.New("component is required")
	}
	channel = normalizeChannel(channel)
	if channel == "" {
		return ChannelAssignment{}, errors.New("channel must be stable, candidate, edge, or lts")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	cur, ok := m.assignments[component]
	if !ok {
		cur = &ChannelAssignment{Component: component}
		m.assignments[component] = cur
	}
	cur.Channel = channel
	cur.UpdatedAt = now
	return *cur, nil
}

func (m *ChannelManager) List() []ChannelAssignment {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ChannelAssignment, 0, len(m.assignments))
	for _, item := range m.assignments {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Component < out[j].Component })
	return out
}

func normalizeChannel(ch string) string {
	switch strings.ToLower(strings.TrimSpace(ch)) {
	case "stable":
		return "stable"
	case "candidate":
		return "candidate"
	case "edge":
		return "edge"
	case "lts":
		return "lts"
	default:
		return ""
	}
}

func ParseControlPlaneProtocol(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 1
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 1
	}
	return v
}

func BuildSupportMatrix(controlPlaneProtocol int) SupportMatrix {
	if controlPlaneProtocol <= 0 {
		controlPlaneProtocol = 1
	}
	minAgent := controlPlaneProtocol - 1
	if minAgent < 1 {
		minAgent = 1
	}
	rows := []SupportMatrixRow{
		{
			Channel:              "stable",
			ControlPlaneProtocol: controlPlaneProtocol,
			MinAgentProtocol:     minAgent,
			MaxAgentProtocol:     controlPlaneProtocol,
			Policy:               "n-1",
		},
		{
			Channel:              "candidate",
			ControlPlaneProtocol: controlPlaneProtocol,
			MinAgentProtocol:     minAgent,
			MaxAgentProtocol:     controlPlaneProtocol,
			Policy:               "n-1",
		},
		{
			Channel:              "edge",
			ControlPlaneProtocol: controlPlaneProtocol,
			MinAgentProtocol:     controlPlaneProtocol,
			MaxAgentProtocol:     controlPlaneProtocol,
			Policy:               "same-major protocol",
		},
		{
			Channel:              "lts",
			ControlPlaneProtocol: controlPlaneProtocol,
			MinAgentProtocol:     minAgent,
			MaxAgentProtocol:     controlPlaneProtocol,
			Policy:               "extended support window",
		},
	}
	return SupportMatrix{
		GeneratedAt:          time.Now().UTC(),
		ControlPlaneProtocol: controlPlaneProtocol,
		Rows:                 rows,
	}
}

func CheckNMinusOneCompatibility(controlPlaneProtocol, agentProtocol int) CompatibilityResult {
	compatible := controlPlaneProtocol > 0 && agentProtocol > 0 && agentProtocol <= controlPlaneProtocol && agentProtocol >= (controlPlaneProtocol-1)
	return CompatibilityResult{
		ControlPlaneProtocol: controlPlaneProtocol,
		AgentProtocol:        agentProtocol,
		Compatible:           compatible,
	}
}
