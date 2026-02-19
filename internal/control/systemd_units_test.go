package control

import "testing"

func TestSystemdUnitStoreUpsertGetRender(t *testing.T) {
	store := NewSystemdUnitStore()
	item, err := store.Upsert(SystemdUnitInput{
		Name:    "payments.service",
		Content: "[Unit]\nDescription=Payments\n[Service]\nExecStart=/usr/local/bin/payments\n",
		DropIns: map[string]string{
			"10-env.conf": "[Service]\nEnvironment=MODE=prod\n",
		},
		Enabled:      true,
		DesiredState: "running",
	})
	if err != nil {
		t.Fatalf("upsert systemd unit failed: %v", err)
	}
	if item.Name != "payments.service" {
		t.Fatalf("unexpected unit %+v", item)
	}
	got, ok := store.Get("payments.service")
	if !ok || got.Name == "" {
		t.Fatalf("expected systemd unit get success")
	}
	rendered, err := store.Render(SystemdRenderInput{Name: "payments.service"})
	if err != nil {
		t.Fatalf("render systemd unit failed: %v", err)
	}
	if rendered.UnitPath == "" || len(rendered.DropInPaths) != 1 {
		t.Fatalf("unexpected render output %+v", rendered)
	}
}

func TestSystemdUnitStoreValidation(t *testing.T) {
	store := NewSystemdUnitStore()
	if _, err := store.Upsert(SystemdUnitInput{
		Name:    "payments.timer",
		Content: "x",
	}); err == nil {
		t.Fatalf("expected service suffix validation error")
	}
	if _, err := store.Render(SystemdRenderInput{Name: "missing.service"}); err == nil {
		t.Fatalf("expected render not found error")
	}
}
