package control

import "testing"

func TestSCIMUpsertRoleAndTeam(t *testing.T) {
	store := NewSCIMStore()
	role, err := store.UpsertRole(SCIMRoleInput{
		ExternalID:  "role-ext-1",
		Name:        "Platform Engineer",
		Description: "platform access",
	})
	if err != nil {
		t.Fatalf("upsert role failed: %v", err)
	}
	if role.ID == "" {
		t.Fatalf("expected role id")
	}

	team, err := store.UpsertTeam(SCIMTeamInput{
		ExternalID: "team-ext-1",
		Name:       "Platform",
		Members:    []string{"alice@example.com", "bob@example.com"},
		Roles:      []string{role.ID},
	})
	if err != nil {
		t.Fatalf("upsert team failed: %v", err)
	}
	if team.ID == "" || len(team.Members) != 2 {
		t.Fatalf("unexpected team: %+v", team)
	}

	team, err = store.UpsertTeam(SCIMTeamInput{
		ExternalID: "team-ext-1",
		Name:       "Platform Core",
		Members:    []string{"alice@example.com"},
		Roles:      []string{role.ID},
	})
	if err != nil {
		t.Fatalf("upsert existing team failed: %v", err)
	}
	if team.Name != "Platform Core" || len(team.Members) != 1 {
		t.Fatalf("expected team update to apply: %+v", team)
	}
}
