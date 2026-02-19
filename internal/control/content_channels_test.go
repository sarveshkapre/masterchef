package control

import "testing"

func TestContentChannelStorePoliciesAndRemotes(t *testing.T) {
	store := NewContentChannelStore()
	channels := store.ListChannels()
	if len(channels) != 3 {
		t.Fatalf("expected 3 channels, got %d", len(channels))
	}

	policy, err := store.SetPolicy(ChannelSyncPolicy{
		Channel:   "validated",
		Allowlist: []string{"module/core/*"},
		Blocklist: []string{"module/community/*"},
	})
	if err != nil {
		t.Fatalf("set policy failed: %v", err)
	}
	if policy.Channel != "validated" || len(policy.Allowlist) != 1 {
		t.Fatalf("unexpected policy: %+v", policy)
	}

	remote, err := store.UpsertRemote(OrgSyncRemoteInput{
		Organization: "acme",
		Channel:      "validated",
		Name:         "central-validated",
		URL:          "https://registry.acme.example/sync",
		APIToken:     "secret-token",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("upsert remote failed: %v", err)
	}
	if remote.ID == "" || !remote.TokenConfigured {
		t.Fatalf("unexpected remote: %+v", remote)
	}

	list := store.ListRemotes("acme", "validated")
	if len(list) != 1 {
		t.Fatalf("expected 1 remote, got %d", len(list))
	}

	rotated, err := store.RotateRemoteToken(remote.ID, "rotated-token")
	if err != nil {
		t.Fatalf("rotate token failed: %v", err)
	}
	if rotated.ID != remote.ID {
		t.Fatalf("expected same remote after rotate")
	}
}
