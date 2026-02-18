package control

import "testing"

func TestMaintenanceStore_SetAndList(t *testing.T) {
	m := NewMaintenanceStore()

	if _, err := m.Set("", "", true, ""); err == nil {
		t.Fatalf("expected validation error for empty kind/name")
	}
	st, err := m.Set("environment", "prod", true, "window")
	if err != nil {
		t.Fatalf("unexpected set error: %v", err)
	}
	if !st.Enabled || st.Kind != "environment" || st.Name != "prod" {
		t.Fatalf("unexpected status: %+v", st)
	}
	if !m.IsActive("environment", "prod") {
		t.Fatalf("expected maintenance target to be active")
	}

	st, err = m.Set("environment", "prod", false, "")
	if err != nil {
		t.Fatalf("unexpected disable error: %v", err)
	}
	if st.Enabled {
		t.Fatalf("expected disabled status")
	}
	if m.IsActive("environment", "prod") {
		t.Fatalf("expected maintenance target to be inactive")
	}

	list := m.List()
	if len(list) != 1 {
		t.Fatalf("expected one maintenance target, got %d", len(list))
	}
}
