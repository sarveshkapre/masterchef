package control

import "testing"

func TestRBACRoleBindingAndAccessCheck(t *testing.T) {
	store := NewRBACStore()
	role, err := store.CreateRole(RBACRoleInput{
		Name: "deployer",
		Permissions: []RBACPermission{
			{Resource: "run", Action: "apply", Scope: "prod"},
			{Resource: "run", Action: "plan", Scope: "*"},
		},
	})
	if err != nil {
		t.Fatalf("create role failed: %v", err)
	}
	if role.ID == "" {
		t.Fatalf("expected role id")
	}

	binding, err := store.CreateBinding(RBACBindingInput{
		Subject: "sre:oncall",
		RoleID:  role.ID,
		Scope:   "prod",
	})
	if err != nil {
		t.Fatalf("create binding failed: %v", err)
	}
	if binding.ID == "" {
		t.Fatalf("expected binding id")
	}

	allowed := store.CheckAccess(RBACAccessCheckInput{
		Subject:  "sre:oncall",
		Resource: "run",
		Action:   "apply",
		Scope:    "prod/service-a",
	})
	if !allowed.Allowed {
		t.Fatalf("expected apply access to be allowed: %+v", allowed)
	}

	denied := store.CheckAccess(RBACAccessCheckInput{
		Subject:  "sre:oncall",
		Resource: "run",
		Action:   "apply",
		Scope:    "staging",
	})
	if denied.Allowed {
		t.Fatalf("expected staging apply access to be denied")
	}
}
