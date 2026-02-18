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
				UntilContains:     "ok",
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected valid retry policy, got %v", err)
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
