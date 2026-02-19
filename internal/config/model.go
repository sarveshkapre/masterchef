package config

// Config is the top-level desired state model for a Masterchef run.
type Config struct {
	Version   string     `json:"version" yaml:"version"`
	Includes  []string   `json:"includes,omitempty" yaml:"includes,omitempty"`
	Imports   []string   `json:"imports,omitempty" yaml:"imports,omitempty"`
	Overlays  []string   `json:"overlays,omitempty" yaml:"overlays,omitempty"`
	Inventory Inventory  `json:"inventory" yaml:"inventory"`
	Execution Execution  `json:"execution,omitempty" yaml:"execution,omitempty"`
	Resources []Resource `json:"resources" yaml:"resources"`
	Handlers  []Resource `json:"handlers,omitempty" yaml:"handlers,omitempty"`
}

type Inventory struct {
	Hosts []Host `json:"hosts" yaml:"hosts"`
}

type Host struct {
	Name         string            `json:"name" yaml:"name"`
	Transport    string            `json:"transport" yaml:"transport"` // local, ssh, winrm
	Address      string            `json:"address,omitempty" yaml:"address,omitempty"`
	User         string            `json:"user,omitempty" yaml:"user,omitempty"`
	Port         int               `json:"port,omitempty" yaml:"port,omitempty"`
	JumpAddress  string            `json:"jump_address,omitempty" yaml:"jump_address,omitempty"`
	JumpUser     string            `json:"jump_user,omitempty" yaml:"jump_user,omitempty"`
	JumpPort     int               `json:"jump_port,omitempty" yaml:"jump_port,omitempty"`
	ProxyCommand string            `json:"proxy_command,omitempty" yaml:"proxy_command,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	Labels       map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Roles        []string          `json:"roles,omitempty" yaml:"roles,omitempty"`
	Topology     map[string]string `json:"topology,omitempty" yaml:"topology,omitempty"`
}

// Resource is a compact typed resource model for v0.
// Type-specific fields are optional and validated by Validate.
type Resource struct {
	ID             string              `json:"id" yaml:"id"`
	Type           string              `json:"type" yaml:"type"` // file, command
	Host           string              `json:"host" yaml:"host"`
	DelegateTo     string              `json:"delegate_to,omitempty" yaml:"delegate_to,omitempty"`
	DependsOn      []string            `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Require        []string            `json:"require,omitempty" yaml:"require,omitempty"`
	Before         []string            `json:"before,omitempty" yaml:"before,omitempty"`
	Notify         []string            `json:"notify,omitempty" yaml:"notify,omitempty"`
	Subscribe      []string            `json:"subscribe,omitempty" yaml:"subscribe,omitempty"`
	NotifyHandlers []string            `json:"notify_handlers,omitempty" yaml:"notify_handlers,omitempty"`
	When           string              `json:"when,omitempty" yaml:"when,omitempty"`
	Matrix         map[string][]string `json:"matrix,omitempty" yaml:"matrix,omitempty"`
	Loop           []string            `json:"loop,omitempty" yaml:"loop,omitempty"`
	LoopVar        string              `json:"loop_var,omitempty" yaml:"loop_var,omitempty"`
	Tags           []string            `json:"tags,omitempty" yaml:"tags,omitempty"`

	// file
	Path    string `json:"path,omitempty" yaml:"path,omitempty"`
	Content string `json:"content,omitempty" yaml:"content,omitempty"`
	Mode    string `json:"mode,omitempty" yaml:"mode,omitempty"`

	// command
	Command           string `json:"command,omitempty" yaml:"command,omitempty"`
	Creates           string `json:"creates,omitempty" yaml:"creates,omitempty"`
	OnlyIf            string `json:"only_if,omitempty" yaml:"only_if,omitempty"`
	Unless            string `json:"unless,omitempty" yaml:"unless,omitempty"`
	RefreshOnly       bool   `json:"refresh_only,omitempty" yaml:"refresh_only,omitempty"`
	RefreshCommand    string `json:"refresh_command,omitempty" yaml:"refresh_command,omitempty"`
	Become            bool   `json:"become,omitempty" yaml:"become,omitempty"`
	BecomeUser        string `json:"become_user,omitempty" yaml:"become_user,omitempty"`
	RescueCommand     string `json:"rescue_command,omitempty" yaml:"rescue_command,omitempty"`
	AlwaysCommand     string `json:"always_command,omitempty" yaml:"always_command,omitempty"`
	Retries           int    `json:"retries,omitempty" yaml:"retries,omitempty"`
	RetryDelaySeconds int    `json:"retry_delay_seconds,omitempty" yaml:"retry_delay_seconds,omitempty"`
	RetryBackoff      string `json:"retry_backoff,omitempty" yaml:"retry_backoff,omitempty"` // constant, linear, exponential
	RetryJitterSecs   int    `json:"retry_jitter_seconds,omitempty" yaml:"retry_jitter_seconds,omitempty"`
	UntilContains     string `json:"until_contains,omitempty" yaml:"until_contains,omitempty"`
}

type Execution struct {
	Strategy          string `json:"strategy,omitempty" yaml:"strategy,omitempty"` // linear|free|serial
	Serial            int    `json:"serial,omitempty" yaml:"serial,omitempty"`     // host batch size for serial strategy
	FailureDomain     string `json:"failure_domain,omitempty" yaml:"failure_domain,omitempty"`
	MaxFailPercentage int    `json:"max_fail_percentage,omitempty" yaml:"max_fail_percentage,omitempty"`
	AnyErrorsFatal    bool   `json:"any_errors_fatal,omitempty" yaml:"any_errors_fatal,omitempty"`
}
