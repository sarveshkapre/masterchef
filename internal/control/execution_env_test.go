package control

import "testing"

func TestExecutionEnvironmentAdmission(t *testing.T) {
	store := NewExecutionEnvironmentStore()
	env, err := store.Create(ExecutionEnvironmentInput{
		Name:         "hermetic-go",
		ImageDigest:  "sha256:9999999999999999999999999999999999999999999999999999999999999999",
		Dependencies: []string{"go1.23.0", "apk@3.20"},
		Signed:       true,
		SignatureRef: "sigstore://artifact/abc",
	})
	if err != nil {
		t.Fatalf("create execution env failed: %v", err)
	}

	policy := store.SetPolicy(ExecutionAdmissionPolicy{
		RequireSigned: true,
		AllowedDigests: []string{
			"sha256:9999999999999999999999999999999999999999999999999999999999999999",
		},
	})
	if !policy.RequireSigned || len(policy.AllowedDigests) != 1 {
		t.Fatalf("unexpected policy %+v", policy)
	}
	allowed := store.EvaluateAdmission(env)
	if !allowed.Allowed {
		t.Fatalf("expected environment to pass admission: %+v", allowed)
	}

	env.Signed = false
	blocked := store.EvaluateAdmission(env)
	if blocked.Allowed {
		t.Fatalf("expected unsigned environment to be blocked")
	}
}
