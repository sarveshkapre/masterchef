package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type NativeSchedulerBackend struct {
	Name               string    `json:"name"`
	DisplayName        string    `json:"display_name"`
	OSFamilies         []string  `json:"os_families"`
	MinIntervalSeconds int       `json:"min_interval_seconds"`
	SupportsJitter     bool      `json:"supports_jitter"`
	SupportsCalendar   bool      `json:"supports_calendar"`
	Priority           int       `json:"priority"`
	Builtin            bool      `json:"builtin"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type NativeSchedulerSelectionRequest struct {
	OSFamily         string `json:"os_family"`
	IntervalSeconds  int    `json:"interval_seconds,omitempty"`
	JitterSeconds    int    `json:"jitter_seconds,omitempty"`
	PreferredBackend string `json:"preferred_backend,omitempty"`
}

type NativeSchedulerSelectionResult struct {
	Supported bool                   `json:"supported"`
	Reason    string                 `json:"reason,omitempty"`
	Backend   NativeSchedulerBackend `json:"backend,omitempty"`
	Fallbacks []string               `json:"fallbacks,omitempty"`
	PlanHint  string                 `json:"plan_hint,omitempty"`
}

type NativeSchedulerCatalog struct {
	mu       sync.RWMutex
	backends map[string]NativeSchedulerBackend
}

func NewNativeSchedulerCatalog() *NativeSchedulerCatalog {
	now := time.Now().UTC()
	backends := map[string]NativeSchedulerBackend{}
	backends["systemd_timer"] = NativeSchedulerBackend{
		Name:               "systemd_timer",
		DisplayName:        "systemd timer",
		OSFamilies:         []string{"linux"},
		MinIntervalSeconds: 30,
		SupportsJitter:     true,
		SupportsCalendar:   true,
		Priority:           100,
		Builtin:            true,
		UpdatedAt:          now,
	}
	backends["cron"] = NativeSchedulerBackend{
		Name:               "cron",
		DisplayName:        "cron",
		OSFamilies:         []string{"linux", "darwin", "bsd"},
		MinIntervalSeconds: 60,
		SupportsJitter:     false,
		SupportsCalendar:   true,
		Priority:           80,
		Builtin:            true,
		UpdatedAt:          now,
	}
	backends["windows_task_scheduler"] = NativeSchedulerBackend{
		Name:               "windows_task_scheduler",
		DisplayName:        "Windows Task Scheduler",
		OSFamilies:         []string{"windows"},
		MinIntervalSeconds: 60,
		SupportsJitter:     false,
		SupportsCalendar:   true,
		Priority:           100,
		Builtin:            true,
		UpdatedAt:          now,
	}
	backends["embedded_scheduler"] = NativeSchedulerBackend{
		Name:               "embedded_scheduler",
		DisplayName:        "Embedded Scheduler (fallback)",
		OSFamilies:         []string{"linux", "darwin", "bsd", "windows"},
		MinIntervalSeconds: 1,
		SupportsJitter:     true,
		SupportsCalendar:   false,
		Priority:           10,
		Builtin:            true,
		UpdatedAt:          now,
	}
	return &NativeSchedulerCatalog{backends: backends}
}

func (c *NativeSchedulerCatalog) List() []NativeSchedulerBackend {
	c.mu.RLock()
	out := make([]NativeSchedulerBackend, 0, len(c.backends))
	for _, item := range c.backends {
		out = append(out, cloneNativeSchedulerBackend(item))
	}
	c.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (c *NativeSchedulerCatalog) Get(name string) (NativeSchedulerBackend, bool) {
	canonical := strings.ToLower(strings.TrimSpace(name))
	if canonical == "" {
		return NativeSchedulerBackend{}, false
	}
	c.mu.RLock()
	item, ok := c.backends[canonical]
	c.mu.RUnlock()
	if !ok {
		return NativeSchedulerBackend{}, false
	}
	return cloneNativeSchedulerBackend(item), true
}

func (c *NativeSchedulerCatalog) Select(req NativeSchedulerSelectionRequest) (NativeSchedulerSelectionResult, error) {
	osFamily := strings.ToLower(strings.TrimSpace(req.OSFamily))
	if osFamily == "" {
		return NativeSchedulerSelectionResult{}, errors.New("os_family is required")
	}
	interval := req.IntervalSeconds
	if interval <= 0 {
		interval = 60
	}
	preferred := strings.ToLower(strings.TrimSpace(req.PreferredBackend))
	if preferred != "" {
		backend, ok := c.Get(preferred)
		if !ok {
			return NativeSchedulerSelectionResult{Supported: false, Reason: "preferred backend not found"}, nil
		}
		if !backendSupportsSelection(backend, osFamily, interval, req.JitterSeconds) {
			return NativeSchedulerSelectionResult{Supported: false, Reason: "preferred backend does not support requested schedule constraints"}, nil
		}
		fallbacks := c.compatibleFallbacks(osFamily, interval, req.JitterSeconds, backend.Name)
		return NativeSchedulerSelectionResult{
			Supported: true,
			Backend:   backend,
			Fallbacks: fallbacks,
			PlanHint:  nativeSchedulerPlanHint(backend.Name, interval),
			Reason:    "preferred backend selected",
		}, nil
	}
	candidates := c.compatibleBackends(osFamily, interval, req.JitterSeconds)
	if len(candidates) == 0 {
		return NativeSchedulerSelectionResult{Supported: false, Reason: "no native scheduler backend supports requested constraints"}, nil
	}
	selected := candidates[0]
	fallbacks := make([]string, 0, len(candidates)-1)
	for i := 1; i < len(candidates); i++ {
		fallbacks = append(fallbacks, candidates[i].Name)
	}
	return NativeSchedulerSelectionResult{
		Supported: true,
		Backend:   selected,
		Fallbacks: fallbacks,
		PlanHint:  nativeSchedulerPlanHint(selected.Name, interval),
		Reason:    "native scheduler selected",
	}, nil
}

func (c *NativeSchedulerCatalog) compatibleBackends(osFamily string, interval, jitter int) []NativeSchedulerBackend {
	all := c.List()
	out := make([]NativeSchedulerBackend, 0, len(all))
	for _, item := range all {
		if backendSupportsSelection(item, osFamily, interval, jitter) {
			out = append(out, item)
		}
	}
	return out
}

func (c *NativeSchedulerCatalog) compatibleFallbacks(osFamily string, interval, jitter int, selected string) []string {
	all := c.compatibleBackends(osFamily, interval, jitter)
	out := make([]string, 0, len(all))
	for _, item := range all {
		if item.Name == selected {
			continue
		}
		out = append(out, item.Name)
	}
	return out
}

func backendSupportsSelection(backend NativeSchedulerBackend, osFamily string, interval, jitter int) bool {
	if !containsStringFold(backend.OSFamilies, osFamily) {
		return false
	}
	if interval < backend.MinIntervalSeconds {
		return false
	}
	if jitter > 0 && !backend.SupportsJitter {
		return false
	}
	return true
}

func nativeSchedulerPlanHint(backend string, interval int) string {
	switch backend {
	case "systemd_timer":
		return "create systemd service/timer units with OnUnitActiveSec=" + itoa(int64(interval)) + "s"
	case "cron":
		return "install cron expression for interval-based execution"
	case "windows_task_scheduler":
		return "register Scheduled Task with schtasks /SC MINUTE"
	case "embedded_scheduler":
		return "fallback to internal scheduler queue for sub-native constraints"
	default:
		return "materialize native scheduler entry"
	}
}

func cloneNativeSchedulerBackend(in NativeSchedulerBackend) NativeSchedulerBackend {
	out := in
	out.OSFamilies = append([]string{}, in.OSFamilies...)
	return out
}
