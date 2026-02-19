package control

import "testing"

func TestProgressiveDisclosureDefaults(t *testing.T) {
	store := NewProgressiveDisclosureStore()
	profiles := store.ListProfiles()
	if len(profiles) != 3 {
		t.Fatalf("expected 3 built-in profiles, got %d", len(profiles))
	}
	state := store.ActiveState()
	if state.ProfileID != "simple" {
		t.Fatalf("expected simple profile by default, got %s", state.ProfileID)
	}
	if len(state.VisibleControls) == 0 {
		t.Fatalf("expected visible controls in default state")
	}
}

func TestProgressiveDisclosureSetProfileAndReveal(t *testing.T) {
	store := NewProgressiveDisclosureStore()
	state, err := store.SetProfile("balanced", "rollout")
	if err != nil {
		t.Fatalf("set profile failed: %v", err)
	}
	if state.ProfileID != "balanced" || state.LastWorkflowHint != "rollout" {
		t.Fatalf("unexpected profile state: %+v", state)
	}

	state, err = store.RevealForWorkflow("rollout", []string{"failure thresholds", "blast radius"})
	if err != nil {
		t.Fatalf("reveal controls failed: %v", err)
	}
	if len(state.RevealedByFlow["rollout"]) != 2 {
		t.Fatalf("expected revealed controls in rollout workflow, got %+v", state.RevealedByFlow)
	}
	found := false
	for _, item := range state.VisibleControls {
		if item == "blast-radius" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected blast-radius in visible controls: %+v", state.VisibleControls)
	}

	if _, err := store.RevealForWorkflow("", []string{"x"}); err == nil {
		t.Fatalf("expected workflow validation error")
	}
}
