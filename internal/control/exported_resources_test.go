package control

import "testing"

func TestExportedResourceStoreCollect(t *testing.T) {
	store := NewExportedResourceStore(100)
	_, err := store.Add(ExportedResourceInput{
		Type:   "service",
		Host:   "node-a",
		Source: "agent",
		Attributes: map[string]string{
			"role": "db",
			"env":  "prod",
		},
	})
	if err != nil {
		t.Fatalf("add first exported resource failed: %v", err)
	}
	_, err = store.Add(ExportedResourceInput{
		Type:   "service",
		Host:   "node-b",
		Source: "agent",
		Attributes: map[string]string{
			"role": "web",
			"env":  "prod",
		},
	})
	if err != nil {
		t.Fatalf("add second exported resource failed: %v", err)
	}

	out, err := store.Collect("type=service and attrs.role=db", 10)
	if err != nil {
		t.Fatalf("collect failed: %v", err)
	}
	if out.Count != 1 || len(out.Items) != 1 {
		t.Fatalf("expected one collected item, got %+v", out)
	}
	if out.Items[0].Host != "node-a" {
		t.Fatalf("expected node-a, got %+v", out.Items[0])
	}
}

func TestExportedResourceStoreCollectRejectsInvalidSelector(t *testing.T) {
	store := NewExportedResourceStore(10)
	if _, err := store.Collect("type", 10); err == nil {
		t.Fatalf("expected invalid selector to fail")
	}
}
