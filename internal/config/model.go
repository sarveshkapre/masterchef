package config

// Config is the top-level desired state model for a Masterchef run.
type Config struct {
	Version   string     `json:"version" yaml:"version"`
	Inventory Inventory  `json:"inventory" yaml:"inventory"`
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
	ID        string   `json:"id" yaml:"id"`
	Type      string   `json:"type" yaml:"type"` // file, command
	Host      string   `json:"host" yaml:"host"`
	DependsOn []string `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`

	// file
	Path    string `json:"path,omitempty" yaml:"path,omitempty"`
	Content string `json:"content,omitempty" yaml:"content,omitempty"`
	Mode    string `json:"mode,omitempty" yaml:"mode,omitempty"`

	// command
	Command string `json:"command,omitempty" yaml:"command,omitempty"`
	Creates string `json:"creates,omitempty" yaml:"creates,omitempty"`
	Unless  string `json:"unless,omitempty" yaml:"unless,omitempty"`
}
