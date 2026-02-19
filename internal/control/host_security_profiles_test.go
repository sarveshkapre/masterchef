package control

import "testing"

func TestHostSecurityProfileStoreUpsertAndEvaluate(t *testing.T) {
	store := NewHostSecurityProfileStore()
	selinux, err := store.Upsert(HostSecurityProfileInput{
		Mode:       "selinux",
		TargetKind: "host",
		Target:     "node-1",
		State:      "enforcing",
		Contexts:   []string{"system_u:object_r:httpd_sys_content_t:s0"},
	})
	if err != nil {
		t.Fatalf("upsert selinux profile failed: %v", err)
	}
	if selinux.ID == "" {
		t.Fatalf("expected selinux profile id")
	}

	allow := store.Evaluate(HostSecurityEvaluateInput{
		Mode:           "selinux",
		TargetKind:     "host",
		Target:         "node-1",
		RequestedState: "enforcing",
	})
	if !allow.Allowed {
		t.Fatalf("expected allow decision, got %+v", allow)
	}

	block := store.Evaluate(HostSecurityEvaluateInput{
		Mode:           "selinux",
		TargetKind:     "host",
		Target:         "node-1",
		RequestedState: "permissive",
	})
	if block.Allowed {
		t.Fatalf("expected enforcing downgrade block, got %+v", block)
	}

	apparmor, err := store.Upsert(HostSecurityProfileInput{
		Mode:       "apparmor",
		TargetKind: "group",
		Target:     "web",
		State:      "complain",
		Profiles:   []string{"usr.sbin.nginx"},
	})
	if err != nil {
		t.Fatalf("upsert apparmor profile failed: %v", err)
	}
	if apparmor.ID == "" {
		t.Fatalf("expected apparmor profile id")
	}
}

func TestHostSecurityProfileStoreValidation(t *testing.T) {
	store := NewHostSecurityProfileStore()
	if _, err := store.Upsert(HostSecurityProfileInput{
		Mode:       "selinux",
		TargetKind: "host",
		Target:     "node-1",
		State:      "complain",
	}); err == nil {
		t.Fatalf("expected invalid selinux state error")
	}
}
