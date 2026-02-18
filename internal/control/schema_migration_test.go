package control

import "testing"

func TestSchemaMigrationManagerCheck(t *testing.T) {
	mgr := NewSchemaMigrationManager(1)

	step := mgr.Check(1, 2)
	if !step.ForwardCompatible {
		t.Fatalf("expected n->n+1 to be forward compatible")
	}
	if step.RequiresMigrationPlan != true {
		t.Fatalf("expected migration plan requirement for version changes")
	}

	rollback := mgr.Check(2, 1)
	if !rollback.BackwardCompatible {
		t.Fatalf("expected n->n-1 to be backward compatible")
	}

	breaking := mgr.Check(1, 3)
	if breaking.ForwardCompatible {
		t.Fatalf("expected n->n+2 to be incompatible")
	}
	if breaking.Reason == "" {
		t.Fatalf("expected incompatibility reason")
	}
}

func TestSchemaMigrationManagerApply(t *testing.T) {
	mgr := NewSchemaMigrationManager(1)

	if _, err := mgr.Apply(1, 2, "", ""); err == nil {
		t.Fatalf("expected missing plan_ref validation error")
	}

	rec, err := mgr.Apply(1, 2, "MIG-001", "initial upgrade")
	if err != nil {
		t.Fatalf("apply migration failed: %v", err)
	}
	if rec.ToVersion != 2 {
		t.Fatalf("unexpected migration record: %+v", rec)
	}
	if st := mgr.Status(); st.CurrentVersion != 2 || len(st.History) != 1 {
		t.Fatalf("unexpected status after migration: %+v", st)
	}

	if _, err := mgr.Apply(2, 4, "MIG-002", "invalid jump"); err == nil {
		t.Fatalf("expected migration jump to fail")
	}
}
