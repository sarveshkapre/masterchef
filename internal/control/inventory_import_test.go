package control

import "testing"

func TestBulkImportFromCMDB(t *testing.T) {
	nodes := NewNodeLifecycleStore()
	res, err := BulkImportFromCMDB(nodes, CMDBImportRequest{
		SourceSystem: "servicenow",
		Records: []CMDBRecord{
			{
				Name:      "node-a",
				Address:   "10.0.0.10",
				Transport: "ssh",
				Labels:    map[string]string{"team": "platform"},
				Roles:     []string{"app"},
			},
			{
				Name:      "node-b",
				Address:   "10.0.0.11",
				Transport: "winrm",
				Roles:     []string{"db"},
			},
		},
	})
	if err != nil {
		t.Fatalf("bulk import failed: %v", err)
	}
	if res.Imported != 2 || res.Updated != 0 || res.Failed != 0 {
		t.Fatalf("unexpected bulk import result %+v", res)
	}

	dryRun, err := BulkImportFromCMDB(nodes, CMDBImportRequest{
		SourceSystem: "servicenow",
		DryRun:       true,
		Records: []CMDBRecord{
			{Name: "node-a"},
			{Name: "node-c"},
		},
	})
	if err != nil {
		t.Fatalf("dry-run bulk import failed: %v", err)
	}
	if dryRun.Imported != 1 || dryRun.Updated != 1 || dryRun.Failed != 0 {
		t.Fatalf("unexpected dry-run result %+v", dryRun)
	}
}

func TestBulkImportFromCMDBValidation(t *testing.T) {
	nodes := NewNodeLifecycleStore()
	if _, err := BulkImportFromCMDB(nodes, CMDBImportRequest{}); err == nil {
		t.Fatalf("expected missing records validation error")
	}
	res, err := BulkImportFromCMDB(nodes, CMDBImportRequest{
		Records: []CMDBRecord{{Name: ""}},
	})
	if err != nil {
		t.Fatalf("unexpected error for invalid record batch: %v", err)
	}
	if res.Failed != 1 {
		t.Fatalf("expected failed item count, got %+v", res)
	}
}
