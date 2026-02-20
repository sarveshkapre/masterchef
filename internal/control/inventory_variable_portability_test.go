package control

import "testing"

func TestBuildInventoryVariableBundle(t *testing.T) {
	baseDir := t.TempDir()

	nodes := NewNodeLifecycleStore()
	if _, _, err := nodes.Enroll(NodeEnrollInput{
		Name:      "node-a",
		Address:   "10.0.0.10",
		Transport: "ssh",
		Source:    "cmdb",
	}); err != nil {
		t.Fatalf("enroll failed: %v", err)
	}
	if _, err := nodes.SetStatus("node-a", NodeStatusActive, "ready"); err != nil {
		t.Fatalf("set status failed: %v", err)
	}

	roleEnv := NewRoleEnvironmentStore(baseDir)
	if _, err := roleEnv.UpsertRole(RoleDefinition{
		Name:    "web",
		RunList: []string{"recipe[web]"},
		DefaultAttributes: map[string]any{
			"region": "us-east-1",
		},
	}); err != nil {
		t.Fatalf("upsert role failed: %v", err)
	}
	if _, err := roleEnv.UpsertEnvironment(EnvironmentDefinition{
		Name: "prod",
		OverrideAttributes: map[string]any{
			"tier": "critical",
		},
	}); err != nil {
		t.Fatalf("upsert environment failed: %v", err)
	}

	encryptedVars := NewEncryptedVariableStore(baseDir)
	if _, err := encryptedVars.Upsert("prod-vars", map[string]any{"api_key": "secret"}, "passphrase"); err != nil {
		t.Fatalf("upsert encrypted vars failed: %v", err)
	}

	bundle, err := BuildInventoryVariableBundle(nodes, roleEnv, encryptedVars, InventoryVariableExportRequest{})
	if err != nil {
		t.Fatalf("export bundle failed: %v", err)
	}
	if bundle.Version != "v1" || bundle.ExportedAt.IsZero() {
		t.Fatalf("unexpected bundle metadata: %+v", bundle)
	}
	if len(bundle.Inventory) != 1 || bundle.Inventory[0].Name != "node-a" {
		t.Fatalf("unexpected inventory export: %#v", bundle.Inventory)
	}
	if len(bundle.Inventory[0].History) != 0 {
		t.Fatalf("expected node history stripped by default, got %#v", bundle.Inventory[0].History)
	}
	if len(bundle.Roles) != 1 || bundle.Roles[0].Name != "web" {
		t.Fatalf("unexpected role export: %#v", bundle.Roles)
	}
	if len(bundle.Environments) != 1 || bundle.Environments[0].Name != "prod" {
		t.Fatalf("unexpected environment export: %#v", bundle.Environments)
	}
	if len(bundle.EncryptedVariableFiles) != 1 || bundle.EncryptedVariableFiles[0].Name != "prod-vars" {
		t.Fatalf("unexpected encrypted variable export: %#v", bundle.EncryptedVariableFiles)
	}
}

func TestImportInventoryVariableBundleDryRunAndApply(t *testing.T) {
	sourceDir := t.TempDir()

	sourceNodes := NewNodeLifecycleStore()
	if _, _, err := sourceNodes.Enroll(NodeEnrollInput{
		Name:      "node-b",
		Address:   "10.0.0.11",
		Transport: "ssh",
		Source:    "servicenow",
	}); err != nil {
		t.Fatalf("source enroll failed: %v", err)
	}
	if _, err := sourceNodes.SetStatus("node-b", NodeStatusQuarantined, "investigation"); err != nil {
		t.Fatalf("source status failed: %v", err)
	}

	sourceRoleEnv := NewRoleEnvironmentStore(sourceDir)
	if _, err := sourceRoleEnv.UpsertRole(RoleDefinition{Name: "db", RunList: []string{"recipe[db]"}}); err != nil {
		t.Fatalf("source role failed: %v", err)
	}
	if _, err := sourceRoleEnv.UpsertEnvironment(EnvironmentDefinition{Name: "stage"}); err != nil {
		t.Fatalf("source environment failed: %v", err)
	}

	sourceVars := NewEncryptedVariableStore(sourceDir)
	if _, err := sourceVars.Upsert("stage-vars", map[string]any{"db_password": "pw"}, "bundle-pass"); err != nil {
		t.Fatalf("source encrypted vars failed: %v", err)
	}

	bundle, err := BuildInventoryVariableBundle(sourceNodes, sourceRoleEnv, sourceVars, InventoryVariableExportRequest{
		IncludeNodeHistory: true,
	})
	if err != nil {
		t.Fatalf("source bundle export failed: %v", err)
	}

	targetDir := t.TempDir()
	targetNodes := NewNodeLifecycleStore()
	targetRoleEnv := NewRoleEnvironmentStore(targetDir)
	targetVars := NewEncryptedVariableStore(targetDir)

	dryRun, err := ImportInventoryVariableBundle(targetNodes, targetRoleEnv, targetVars, InventoryVariableImportRequest{
		DryRun: true,
		Bundle: bundle,
	})
	if err != nil {
		t.Fatalf("dry-run import failed: %v", err)
	}
	if dryRun.Inventory.Imported != 1 || dryRun.Roles.Imported != 1 || dryRun.Environments.Imported != 1 || dryRun.EncryptedVariableFiles.Imported != 1 {
		t.Fatalf("unexpected dry-run result: %+v", dryRun)
	}

	apply, err := ImportInventoryVariableBundle(targetNodes, targetRoleEnv, targetVars, InventoryVariableImportRequest{
		Bundle: bundle,
	})
	if err != nil {
		t.Fatalf("apply import failed: %v", err)
	}
	if apply.Inventory.Imported != 1 || apply.Inventory.Failed != 0 {
		t.Fatalf("unexpected inventory apply result: %+v", apply.Inventory)
	}
	node, ok := targetNodes.Get("node-b")
	if !ok {
		t.Fatalf("expected imported node")
	}
	if node.Status != NodeStatusQuarantined {
		t.Fatalf("expected imported node status %q, got %q", NodeStatusQuarantined, node.Status)
	}
	if _, err := targetRoleEnv.GetRole("db"); err != nil {
		t.Fatalf("expected imported role: %v", err)
	}
	if _, err := targetRoleEnv.GetEnvironment("stage"); err != nil {
		t.Fatalf("expected imported environment: %v", err)
	}
	if _, _, err := targetVars.Get("stage-vars", "bundle-pass"); err != nil {
		t.Fatalf("expected imported encrypted variable file to decrypt: %v", err)
	}
}
