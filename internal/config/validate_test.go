package config

import "testing"

func TestValidate_OK(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{ID: "f1", Type: "file", Host: "localhost", Path: "/tmp/x", Content: "ok"},
			{ID: "c1", Type: "command", Host: "localhost", DependsOn: []string{"f1"}, Command: "echo ok"},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestValidate_CycleRefFailsUnknownDependency(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{ID: "a", Type: "file", Host: "localhost", Path: "/tmp/a", DependsOn: []string{"missing"}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected validation error for missing dependency")
	}
}

func TestValidate_ExecutionPolicy(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Execution: Execution{
			Strategy:          "free",
			FailureDomain:     "zone",
			MaxFailPercentage: 25,
		},
		Resources: []Resource{
			{ID: "f1", Type: "file", Host: "localhost", Path: "/tmp/x"},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected valid execution policy, got %v", err)
	}
	cfg.Execution.Strategy = "invalid"
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected invalid strategy error")
	}
	cfg.Execution.Strategy = "linear"
	cfg.Execution.FailureDomain = "invalid"
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected failure_domain validation error")
	}
	cfg.Execution.FailureDomain = "zone"
	cfg.Execution.MaxFailPercentage = 200
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected max_fail_percentage validation error")
	}
}

func TestValidate_NormalizesResourceTags(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{ID: "f1", Type: "file", Host: "localhost", Path: "/tmp/x", Tags: []string{"Prod", "prod", " api ", ""}},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if len(cfg.Resources[0].Tags) != 2 || cfg.Resources[0].Tags[0] != "api" || cfg.Resources[0].Tags[1] != "prod" {
		t.Fatalf("expected normalized sorted deduped tags, got %#v", cfg.Resources[0].Tags)
	}
}

func TestValidate_CommandRetryPolicy(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{
				ID:                "c1",
				Type:              "command",
				Host:              "localhost",
				Command:           "echo ok",
				Retries:           2,
				RetryDelaySeconds: 1,
				RetryBackoff:      "LINEAR",
				RetryJitterSecs:   1,
				UntilContains:     "ok",
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected valid retry policy, got %v", err)
	}
	if cfg.Resources[0].RetryBackoff != "linear" {
		t.Fatalf("expected normalized retry backoff, got %q", cfg.Resources[0].RetryBackoff)
	}
	cfg.Resources[0].Retries = -1
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected retries validation error")
	}
	cfg.Resources[0].Retries = 0
	cfg.Resources[0].RetryDelaySeconds = -1
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected retry delay validation error")
	}
	cfg.Resources[0].RetryDelaySeconds = 1
	cfg.Resources[0].RetryBackoff = "invalid"
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected retry backoff validation error")
	}
	cfg.Resources[0].RetryBackoff = "constant"
	cfg.Resources[0].RetryJitterSecs = -1
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected retry jitter validation error")
	}
}

func TestValidate_PrivilegeEscalationCommandOnly(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{
				ID:         "c1",
				Type:       "command",
				Host:       "localhost",
				Command:    "echo ok",
				BecomeUser: "root",
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected command privilege escalation to validate, got %v", err)
	}
	if !cfg.Resources[0].Become || cfg.Resources[0].BecomeUser != "root" {
		t.Fatalf("expected become normalization, got %#v", cfg.Resources[0])
	}
	cfg.Resources[0] = Resource{
		ID:     "f1",
		Type:   "file",
		Host:   "localhost",
		Path:   "/tmp/x",
		Become: true,
	}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected privilege escalation on file resource to fail")
	}
}

func TestValidate_BlockRescueAlwaysCommandOnly(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{
				ID:            "c1",
				Type:          "command",
				Host:          "localhost",
				Command:       "echo ok",
				RescueCommand: "echo rescued",
				AlwaysCommand: "echo always",
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected command hooks to validate, got %v", err)
	}
	cfg.Resources[0] = Resource{
		ID:            "f1",
		Type:          "file",
		Host:          "localhost",
		Path:          "/tmp/x",
		RescueCommand: "echo no",
	}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected file hook validation error")
	}
}

func TestValidate_DelegateToHost(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{
				{Name: "target", Transport: "local"},
				{Name: "delegate", Transport: "local"},
			},
		},
		Resources: []Resource{
			{ID: "c1", Type: "command", Host: "target", DelegateTo: "delegate", Command: "echo ok"},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected valid delegate_to host, got %v", err)
	}
	cfg.Resources[0].DelegateTo = "missing"
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected delegate_to validation error")
	}
}

func TestValidate_NormalizesHostMetadata(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{
				{
					Name:      "h1",
					Transport: "local",
					Roles:     []string{"App", " app ", "DB"},
					Labels: map[string]string{
						" Team ": "platform",
					},
					Topology: map[string]string{
						" Zone ": "us-east-1a",
					},
				},
			},
		},
		Resources: []Resource{
			{ID: "f1", Type: "file", Host: "h1", Path: "/tmp/x"},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	host := cfg.Inventory.Hosts[0]
	if len(host.Roles) != 2 || host.Roles[0] != "app" || host.Roles[1] != "db" {
		t.Fatalf("expected normalized roles, got %#v", host.Roles)
	}
	if host.Labels["team"] != "platform" {
		t.Fatalf("expected normalized labels map key, got %#v", host.Labels)
	}
	if host.Topology["zone"] != "us-east-1a" {
		t.Fatalf("expected normalized topology key, got %#v", host.Topology)
	}
}

func TestValidate_HostConnectionRoutingFields(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{
				{
					Name:         "edge-1",
					Transport:    "ssh",
					Port:         22,
					JumpAddress:  " bastion.internal ",
					JumpUser:     "  ops ",
					JumpPort:     2222,
					ProxyCommand: " nc -x proxy.internal:1080 %h %p ",
				},
			},
		},
		Resources: []Resource{
			{ID: "c1", Type: "command", Host: "edge-1", Command: "echo ok"},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected valid host connection routing settings, got %v", err)
	}
	host := cfg.Inventory.Hosts[0]
	if host.JumpAddress != "bastion.internal" || host.JumpUser != "ops" {
		t.Fatalf("expected trimmed jump settings, got %#v", host)
	}
	if host.ProxyCommand != "nc -x proxy.internal:1080 %h %p" {
		t.Fatalf("expected trimmed proxy_command, got %q", host.ProxyCommand)
	}

	cfg.Inventory.Hosts[0].JumpPort = 70000
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected jump_port validation error")
	}
	cfg.Inventory.Hosts[0].JumpPort = 2222
	cfg.Inventory.Hosts[0].Port = -1
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected port validation error")
	}
}

func TestValidate_AllowsPluginTransports(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{
				{Name: "custom-1", Transport: "plugin/mock"},
			},
		},
		Resources: []Resource{
			{ID: "f1", Type: "file", Host: "custom-1", Path: "/tmp/x"},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected plugin transport to be allowed, got %v", err)
	}
}

func TestValidate_AutoTransportCapabilities(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{
				{
					Name:         "node-1",
					Transport:    "AUTO",
					Capabilities: []string{"ssh", " winrm ", "ssh"},
				},
			},
		},
		Resources: []Resource{
			{ID: "c1", Type: "command", Host: "node-1", Command: "echo ok"},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected auto transport and capabilities to validate, got %v", err)
	}
	if cfg.Inventory.Hosts[0].Transport != "auto" {
		t.Fatalf("expected normalized auto transport, got %q", cfg.Inventory.Hosts[0].Transport)
	}
	if len(cfg.Inventory.Hosts[0].Capabilities) != 2 || cfg.Inventory.Hosts[0].Capabilities[0] != "ssh" || cfg.Inventory.Hosts[0].Capabilities[1] != "winrm" {
		t.Fatalf("expected normalized capabilities, got %#v", cfg.Inventory.Hosts[0].Capabilities)
	}

	cfg.Inventory.Hosts[0].Capabilities = []string{"ssh", "unknown"}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected unsupported capability validation error")
	}
}

func TestValidate_ResourceRelationshipReferences(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{ID: "a", Type: "file", Host: "localhost", Path: "/tmp/a"},
			{ID: "b", Type: "command", Host: "localhost", Command: "echo b", Require: []string{"a"}, Subscribe: []string{"a"}},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected relationship references to validate, got %v", err)
	}
	cfg.Resources[1].Require = []string{"missing"}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected missing require reference validation error")
	}
}

func TestValidate_RefreshOnlyRequiresTrigger(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{ID: "svc", Type: "command", Host: "localhost", Command: "echo restart", RefreshOnly: true},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected refresh_only without trigger to fail validation")
	}
	cfg.Resources = []Resource{
		{ID: "cfg", Type: "file", Host: "localhost", Path: "/tmp/cfg"},
		{ID: "svc", Type: "command", Host: "localhost", Command: "echo restart", RefreshOnly: true, Subscribe: []string{"cfg"}},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected refresh_only with subscribe trigger to validate, got %v", err)
	}
}

func TestValidate_FileGuardsAndRefreshCommandRejected(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{ID: "f1", Type: "file", Host: "localhost", Path: "/tmp/f1", OnlyIf: "test -f /tmp/gate"},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected file only_if guard to fail validation")
	}
	cfg.Resources[0] = Resource{ID: "f1", Type: "file", Host: "localhost", Path: "/tmp/f1", RefreshCommand: "echo no"}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected file refresh_command to fail validation")
	}
}

func TestValidate_HandlersAndNotifyHandlers(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{
				ID:             "cfg",
				Type:           "file",
				Host:           "localhost",
				Path:           "/tmp/cfg",
				NotifyHandlers: []string{"restart-app"},
			},
		},
		Handlers: []Resource{
			{
				ID:      "restart-app",
				Type:    "command",
				Host:    "localhost",
				Command: "echo restart",
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected handlers+notify_handlers to validate, got %v", err)
	}
	cfg.Resources[0].NotifyHandlers = []string{"missing"}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected unknown notify handler validation error")
	}
}

func TestValidate_HandlerGraphRelationsRejected(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{ID: "f1", Type: "file", Host: "localhost", Path: "/tmp/x"},
		},
		Handlers: []Resource{
			{ID: "h1", Type: "command", Host: "localhost", Command: "echo hi", DependsOn: []string{"f1"}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected handler relation validation error")
	}
}

func TestValidate_MatrixNormalizationAndValidation(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{
				ID:      "m1",
				Type:    "command",
				Host:    "localhost",
				Command: "echo hi",
				Matrix: map[string][]string{
					" env ": []string{"prod", "prod", " staging "},
				},
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected matrix to normalize and validate, got %v", err)
	}
	values := cfg.Resources[0].Matrix["env"]
	if len(values) != 2 || values[0] != "prod" || values[1] != "staging" {
		t.Fatalf("expected normalized matrix values, got %#v", cfg.Resources[0].Matrix)
	}

	cfg.Resources[0].Matrix = map[string][]string{"": []string{"x"}}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected empty matrix key validation error")
	}
	cfg.Resources[0].Matrix = map[string][]string{"region": []string{" ", ""}}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected empty matrix values validation error")
	}
}

func TestValidate_LoopNormalizationAndValidation(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{
				ID:      "l1",
				Type:    "command",
				Host:    "localhost",
				Command: "echo hi",
				Loop:    []string{"prod", "prod", " staging ", ""},
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected loop to normalize and validate, got %v", err)
	}
	if cfg.Resources[0].LoopVar != "item" {
		t.Fatalf("expected default loop_var=item, got %q", cfg.Resources[0].LoopVar)
	}
	if len(cfg.Resources[0].Loop) != 2 || cfg.Resources[0].Loop[0] != "prod" || cfg.Resources[0].Loop[1] != "staging" {
		t.Fatalf("expected normalized loop values, got %#v", cfg.Resources[0].Loop)
	}

	cfg.Resources[0].LoopVar = "env.value"
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected invalid loop_var validation error")
	}
	cfg.Resources[0].LoopVar = "env"
	cfg.Resources[0].Matrix = map[string][]string{"env": []string{"prod"}}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected loop_var matrix conflict validation error")
	}
}

func TestValidate_RetryBackoffCommandOnly(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{
				ID:              "f1",
				Type:            "file",
				Host:            "localhost",
				Path:            "/tmp/x",
				RetryBackoff:    "linear",
				RetryJitterSecs: 1,
			},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected retry backoff/jitter on file resource to fail")
	}
}

func TestValidate_WindowsResourceTypes(t *testing.T) {
	cfg := &Config{
		Version: "v0",
		Inventory: Inventory{
			Hosts: []Host{{Name: "localhost", Transport: "local"}},
		},
		Resources: []Resource{
			{
				ID:                "reg-1",
				Type:              "registry",
				Host:              "localhost",
				RegistryKey:       `hkcu\\software\\masterchef\\setting`,
				RegistryValue:     "enabled",
				RegistryValueType: "STRING",
			},
			{
				ID:          "task-1",
				Type:        "scheduled_task",
				Host:        "localhost",
				TaskName:    "nightly-cleanup",
				TaskCommand: "echo cleanup",
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected windows resources to validate, got %v", err)
	}
	if cfg.Resources[0].RegistryValueType != "string" {
		t.Fatalf("expected normalized registry value type, got %q", cfg.Resources[0].RegistryValueType)
	}
	if cfg.Resources[1].TaskSchedule != "@daily" {
		t.Fatalf("expected default task schedule, got %q", cfg.Resources[1].TaskSchedule)
	}

	cfg.Resources[0].RegistryValueType = "binary"
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected invalid registry value type error")
	}
	cfg.Resources[0].RegistryValueType = "string"
	cfg.Resources[1].TaskCommand = ""
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected missing task command error")
	}
}
