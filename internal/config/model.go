package config

// Config is the top-level desired state model for a Masterchef run.
type Config struct {
	Version   string     `json:"version" yaml:"version"`
	Inventory Inventory  `json:"inventory" yaml:"inventory"`
	Execution Execution  `json:"execution,omitempty" yaml:"execution,omitempty"`
	Resources []Resource `json:"resources" yaml:"resources"`
}

type Inventory struct {
	Hosts []Host `json:"hosts" yaml:"hosts"`
}

type Host struct {
	Name      string `json:"name" yaml:"name"`
	Transport string `json:"transport" yaml:"transport"` // local, ssh, winrm
	Address   string `json:"address,omitempty" yaml:"address,omitempty"`
	User      string `json:"user,omitempty" yaml:"user,omitempty"`
	Port      int    `json:"port,omitempty" yaml:"port,omitempty"`
}

// Resource is a compact typed resource model for v0.
// Type-specific fields are optional and validated by Validate.
type Resource struct {
	ID         string   `json:"id" yaml:"id"`
	Type       string   `json:"type" yaml:"type"` // file, command
	Host       string   `json:"host" yaml:"host"`
	DelegateTo string   `json:"delegate_to,omitempty" yaml:"delegate_to,omitempty"`
	DependsOn  []string `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Tags       []string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// file
	Path    string `json:"path,omitempty" yaml:"path,omitempty"`
	Content string `json:"content,omitempty" yaml:"content,omitempty"`
	Mode    string `json:"mode,omitempty" yaml:"mode,omitempty"`

	// command
	Command           string `json:"command,omitempty" yaml:"command,omitempty"`
	Creates           string `json:"creates,omitempty" yaml:"creates,omitempty"`
	Unless            string `json:"unless,omitempty" yaml:"unless,omitempty"`
	Retries           int    `json:"retries,omitempty" yaml:"retries,omitempty"`
	RetryDelaySeconds int    `json:"retry_delay_seconds,omitempty" yaml:"retry_delay_seconds,omitempty"`
	UntilContains     string `json:"until_contains,omitempty" yaml:"until_contains,omitempty"`
}

type Execution struct {
	Strategy          string `json:"strategy,omitempty" yaml:"strategy,omitempty"` // linear|free|serial
	Serial            int    `json:"serial,omitempty" yaml:"serial,omitempty"`     // host batch size for serial strategy
	MaxFailPercentage int    `json:"max_fail_percentage,omitempty" yaml:"max_fail_percentage,omitempty"`
	AnyErrorsFatal    bool   `json:"any_errors_fatal,omitempty" yaml:"any_errors_fatal,omitempty"`
}
