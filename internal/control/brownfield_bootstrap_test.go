package control

import "testing"

func TestBuildBrownfieldBaseline(t *testing.T) {
	out, err := BuildBrownfieldBaseline(BrownfieldBootstrapRequest{
		Hosts: []BrownfieldObservedHost{
			{
				Name:      "node-a",
				Address:   "10.0.0.5",
				Transport: "ssh",
				Packages:  []string{"nginx"},
				Services:  []string{"nginx"},
				Files:     []string{"/etc/nginx/nginx.conf"},
			},
		},
	})
	if err != nil {
		t.Fatalf("build brownfield baseline failed: %v", err)
	}
	if out.Counts["hosts"] != 1 || out.Counts["resources"] != 3 {
		t.Fatalf("unexpected counts %+v", out.Counts)
	}
	if len(out.InventoryHosts) != 1 || len(out.Resources) != 3 {
		t.Fatalf("unexpected output sizes: hosts=%d resources=%d", len(out.InventoryHosts), len(out.Resources))
	}
}

func TestBuildBrownfieldBaselineValidation(t *testing.T) {
	if _, err := BuildBrownfieldBaseline(BrownfieldBootstrapRequest{}); err == nil {
		t.Fatalf("expected empty hosts validation error")
	}
	if _, err := BuildBrownfieldBaseline(BrownfieldBootstrapRequest{
		Hosts: []BrownfieldObservedHost{{Name: ""}},
	}); err == nil {
		t.Fatalf("expected host name validation error")
	}
}
