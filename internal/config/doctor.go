package config

import "fmt"

type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

type Diagnostic struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
}

func Analyze(cfg *Config) []Diagnostic {
	diags := make([]Diagnostic, 0)
	if cfg == nil {
		return append(diags, Diagnostic{Severity: SeverityError, Code: "CFG_NIL", Message: "config is nil"})
	}
	if cfg.Version != "v0" {
		diags = append(diags, Diagnostic{Severity: SeverityWarn, Code: "CFG_VERSION", Message: fmt.Sprintf("config version %q is not current stable version v0", cfg.Version)})
	}
	for i, h := range cfg.Inventory.Hosts {
		if h.Transport == "ssh" {
			if h.Address == "" {
				diags = append(diags, Diagnostic{Severity: SeverityWarn, Code: "HOST_SSH_ADDRESS", Message: fmt.Sprintf("inventory.hosts[%d] ssh host should set address", i)})
			}
			if h.User == "" {
				diags = append(diags, Diagnostic{Severity: SeverityInfo, Code: "HOST_SSH_USER", Message: fmt.Sprintf("inventory.hosts[%d] ssh host should set user explicitly", i)})
			}
		}
	}
	for i, r := range cfg.Resources {
		switch r.Type {
		case "command":
			if r.Creates == "" && r.Unless == "" {
				diags = append(diags, Diagnostic{Severity: SeverityWarn, Code: "CMD_NON_IDEMPOTENT", Message: fmt.Sprintf("resources[%d] command should set creates or unless for idempotency", i)})
			}
		case "file":
			if r.Mode == "" {
				diags = append(diags, Diagnostic{Severity: SeverityInfo, Code: "FILE_MODE_UNSET", Message: fmt.Sprintf("resources[%d] file does not set mode explicitly", i)})
			}
		}
	}
	return diags
}
