package control

import "testing"

func TestNetworkTransportCatalogDefaultsAndValidate(t *testing.T) {
	catalog := NewNetworkTransportCatalog()
	items := catalog.List()
	if len(items) < 3 {
		t.Fatalf("expected builtin transports, got %d", len(items))
	}
	result, err := catalog.Validate("NETCONF")
	if err != nil {
		t.Fatalf("validate netconf: %v", err)
	}
	if !result.Supported || result.Canonical != "netconf" || result.Category != "netconf" {
		t.Fatalf("unexpected validation %+v", result)
	}

	plugin, err := catalog.Validate("plugin/vendor-x")
	if err != nil {
		t.Fatalf("validate plugin: %v", err)
	}
	if !plugin.Supported || plugin.Builtin || plugin.Category != "plugin" {
		t.Fatalf("unexpected plugin validation %+v", plugin)
	}

	unsupported, err := catalog.Validate("snmp")
	if err != nil {
		t.Fatalf("validate unsupported: %v", err)
	}
	if unsupported.Supported {
		t.Fatalf("expected unsupported transport")
	}
}

func TestNetworkTransportCatalogRegister(t *testing.T) {
	catalog := NewNetworkTransportCatalog()
	item, err := catalog.Register(NetworkTransportInput{
		Name:                     "eapi",
		Description:              "Arista eAPI",
		Category:                 "api",
		DefaultPort:              443,
		SupportsConfigPush:       true,
		SupportsTelemetryPull:    true,
		CredentialFieldsRequired: []string{"endpoint", "username", "password"},
		Metadata:                 map[string]string{"vendor": "arista"},
	})
	if err != nil {
		t.Fatalf("register custom transport: %v", err)
	}
	if item.Name != "eapi" || item.Builtin {
		t.Fatalf("unexpected registered item %+v", item)
	}
	validated, err := catalog.Validate("eapi")
	if err != nil {
		t.Fatalf("validate custom transport: %v", err)
	}
	if !validated.Supported || validated.Category != "api" {
		t.Fatalf("unexpected validation %+v", validated)
	}
}
