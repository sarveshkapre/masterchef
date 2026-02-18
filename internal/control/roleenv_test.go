package control

import "testing"

func TestRoleEnvironmentStorePersistsAndLoads(t *testing.T) {
	baseDir := t.TempDir()
	store := NewRoleEnvironmentStore(baseDir)

	role, err := store.UpsertRole(RoleDefinition{
		Name:        "web",
		Description: "web role",
		RunList:     []string{"recipe[base]", "recipe[web]"},
		PolicyGroup: "stable",
		DefaultAttributes: map[string]any{
			"region": "us-east-1",
		},
	})
	if err != nil {
		t.Fatalf("upsert role failed: %v", err)
	}
	if role.Name != "web" {
		t.Fatalf("expected normalized role name, got %s", role.Name)
	}

	env, err := store.UpsertEnvironment(EnvironmentDefinition{
		Name:        "prod",
		Description: "production",
		RunListOverrides: map[string][]string{
			"web": {"recipe[base]", "recipe[web-hardening]"},
		},
	})
	if err != nil {
		t.Fatalf("upsert environment failed: %v", err)
	}
	if env.Name != "prod" {
		t.Fatalf("expected normalized environment name, got %s", env.Name)
	}

	reloaded := NewRoleEnvironmentStore(baseDir)
	gotRole, err := reloaded.GetRole("web")
	if err != nil {
		t.Fatalf("reloaded role get failed: %v", err)
	}
	if len(gotRole.RunList) != 2 {
		t.Fatalf("expected role run list to persist, got %#v", gotRole.RunList)
	}
	gotEnv, err := reloaded.GetEnvironment("prod")
	if err != nil {
		t.Fatalf("reloaded environment get failed: %v", err)
	}
	if len(gotEnv.RunListOverrides["web"]) != 2 {
		t.Fatalf("expected environment override to persist, got %#v", gotEnv.RunListOverrides)
	}
}

func TestRoleEnvironmentResolvePrecedence(t *testing.T) {
	baseDir := t.TempDir()
	store := NewRoleEnvironmentStore(baseDir)

	_, err := store.UpsertRole(RoleDefinition{
		Name:    "app",
		RunList: []string{"recipe[base]", "recipe[app]"},
		DefaultAttributes: map[string]any{
			"level": "role-default",
			"nested": map[string]any{
				"a": "one",
				"b": "two",
			},
		},
		OverrideAttributes: map[string]any{
			"nested": map[string]any{
				"b": "role-override",
			},
			"role_only": true,
		},
	})
	if err != nil {
		t.Fatalf("upsert role failed: %v", err)
	}
	_, err = store.UpsertEnvironment(EnvironmentDefinition{
		Name: "prod",
		DefaultAttributes: map[string]any{
			"level": "env-default",
			"nested": map[string]any{
				"c": "three",
			},
		},
		OverrideAttributes: map[string]any{
			"level": "env-override",
		},
		PolicyOverrides: map[string]any{
			"policy_only": "yes",
		},
		RunListOverrides: map[string][]string{
			"app": {"recipe[base]", "recipe[app-prod]"},
		},
	})
	if err != nil {
		t.Fatalf("upsert environment failed: %v", err)
	}

	res, err := store.Resolve("app", "prod")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(res.RunList) != 2 || res.RunList[1] != "recipe[app-prod]" {
		t.Fatalf("expected run list override, got %#v", res.RunList)
	}
	if res.Attributes["level"] != "env-override" {
		t.Fatalf("expected env override precedence, got %#v", res.Attributes["level"])
	}
	if res.Attributes["policy_only"] != "yes" || res.Attributes["role_only"] != true {
		t.Fatalf("expected merged policy+role fields, got %#v", res.Attributes)
	}
	nested, ok := res.Attributes["nested"].(map[string]any)
	if !ok {
		t.Fatalf("expected merged nested map, got %#v", res.Attributes["nested"])
	}
	if nested["a"] != "one" || nested["b"] != "role-override" || nested["c"] != "three" {
		t.Fatalf("unexpected nested merge: %#v", nested)
	}
}

func TestRoleEnvironmentDelete(t *testing.T) {
	baseDir := t.TempDir()
	store := NewRoleEnvironmentStore(baseDir)
	_, _ = store.UpsertRole(RoleDefinition{Name: "db"})
	_, _ = store.UpsertEnvironment(EnvironmentDefinition{Name: "staging"})

	if !store.DeleteRole("db") {
		t.Fatalf("expected role delete to return true")
	}
	if !store.DeleteEnvironment("staging") {
		t.Fatalf("expected environment delete to return true")
	}
	if store.DeleteRole("db") {
		t.Fatalf("expected second role delete to return false")
	}
	if store.DeleteEnvironment("staging") {
		t.Fatalf("expected second environment delete to return false")
	}
}
