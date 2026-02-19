package control

import "testing"

func TestABACPolicyCheck(t *testing.T) {
	store := NewABACStore()
	_, err := store.CreatePolicy(ABACPolicyInput{
		Name:     "deny-freeze",
		Effect:   "deny",
		Subject:  "sre:oncall",
		Resource: "run",
		Action:   "apply",
		Conditions: map[string]string{
			"freeze_active": "true",
		},
		Priority: 100,
	})
	if err != nil {
		t.Fatalf("create deny policy failed: %v", err)
	}
	_, err = store.CreatePolicy(ABACPolicyInput{
		Name:     "allow-prod",
		Effect:   "allow",
		Subject:  "sre:oncall",
		Resource: "run",
		Action:   "apply",
		Conditions: map[string]string{
			"environment": "prod",
		},
		Priority: 10,
	})
	if err != nil {
		t.Fatalf("create allow policy failed: %v", err)
	}

	denied := store.Check(ABACCheckInput{
		Subject:  "sre:oncall",
		Resource: "run",
		Action:   "apply",
		Context: map[string]string{
			"freeze_active": "true",
			"environment":   "prod",
		},
	})
	if denied.Allowed {
		t.Fatalf("expected deny policy to block access: %+v", denied)
	}

	allowed := store.Check(ABACCheckInput{
		Subject:  "sre:oncall",
		Resource: "run",
		Action:   "apply",
		Context: map[string]string{
			"freeze_active": "false",
			"environment":   "prod",
		},
	})
	if !allowed.Allowed {
		t.Fatalf("expected allow policy to permit access: %+v", allowed)
	}
}
