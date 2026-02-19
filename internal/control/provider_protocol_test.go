package control

import "testing"

func TestProviderProtocolNegotiation(t *testing.T) {
	store := NewProviderProtocolStore()
	okResult, err := store.Negotiate(ProviderNegotiationInput{
		Provider:              "file",
		ControllerVersion:     "v1.5",
		RequestedCapabilities: []string{"idempotent_apply"},
		RequiredFeatureFlags:  []string{"signed-content"},
	})
	if err != nil {
		t.Fatalf("negotiate compatible provider failed: %v", err)
	}
	if !okResult.Compatible {
		t.Fatalf("expected compatible result, got %+v", okResult)
	}

	badResult, err := store.Negotiate(ProviderNegotiationInput{
		Provider:              "file",
		ControllerVersion:     "v1.5",
		RequestedCapabilities: []string{"not-supported"},
		RequiredFeatureFlags:  []string{"not-a-flag"},
	})
	if err != nil {
		t.Fatalf("negotiate incompatible provider failed: %v", err)
	}
	if badResult.Compatible {
		t.Fatalf("expected incompatible result, got %+v", badResult)
	}
}
