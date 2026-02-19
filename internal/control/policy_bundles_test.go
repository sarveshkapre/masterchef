package control

import "testing"

func TestPolicyBundleStore_CreateAndPromote(t *testing.T) {
	store := NewPolicyBundleStore()
	bundle, err := store.Create(PolicyBundleInput{
		Name:        "base-linux",
		Version:     "1.2.3",
		PolicyGroup: "candidate",
		RunList:     []string{"profile::base", "recipe[hardening]"},
		LockEntries: []PolicyLockEntry{
			{Name: "nginx", Version: "1.24.0", Digest: "sha256:aaa", Source: "internal"},
			{Name: "openssl", Version: "3.1.4", Digest: "sha256:bbb", Source: "internal"},
		},
	})
	if err != nil {
		t.Fatalf("create bundle failed: %v", err)
	}
	if bundle.ID == "" {
		t.Fatalf("expected bundle id")
	}
	if bundle.LockDigest == "" {
		t.Fatalf("expected lock digest to be computed")
	}

	promo, err := store.Promote(bundle.ID, PolicyBundlePromotionInput{TargetGroup: "stable"})
	if err != nil {
		t.Fatalf("promote failed: %v", err)
	}
	if promo.BundleID != bundle.ID || promo.TargetGroup != "stable" {
		t.Fatalf("unexpected promotion %+v", promo)
	}
	if len(promo.RunList) != 2 {
		t.Fatalf("expected promotion run_list to inherit bundle runlist, got %+v", promo.RunList)
	}

	updated, ok := store.Get(bundle.ID)
	if !ok {
		t.Fatalf("expected promoted bundle to exist")
	}
	if updated.PolicyGroup != "stable" {
		t.Fatalf("expected bundle policy group to update on promote, got %q", updated.PolicyGroup)
	}

	promotions := store.ListPromotions(bundle.ID)
	if len(promotions) != 1 {
		t.Fatalf("expected one promotion record, got %d", len(promotions))
	}
}

func TestPolicyBundleStore_ValidatesLockEntries(t *testing.T) {
	store := NewPolicyBundleStore()
	_, err := store.Create(PolicyBundleInput{
		Name:    "bad",
		Version: "1.0.0",
		LockEntries: []PolicyLockEntry{
			{Name: "nginx", Version: "1.0", Digest: "sha256:1"},
			{Name: "nginx", Version: "1.0", Digest: "sha256:2"},
		},
	})
	if err == nil {
		t.Fatalf("expected duplicate lock entry validation error")
	}
}
