package features

import "testing"

func TestVerify_StrictPassesForRepositoryFeaturesDoc(t *testing.T) {
	doc, err := Parse("../../features.md")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	report := Verify(doc)
	if !report.TraceabilityIsStrict {
		t.Fatalf("expected strict report, got: %#v", report)
	}
}

func TestVerify_DetectsMismatch(t *testing.T) {
	doc := &Document{
		Bullets: []string{"foo"},
		Matrix: []MatrixRow{
			{ID: "X-1", Competitor: "Chef", Feature: "f", Mapping: "missing"},
			{ID: "X-1", Competitor: "Ansible", Feature: "f", Mapping: "foo"},
		},
	}
	report := Verify(doc)
	if report.TraceabilityIsStrict {
		t.Fatalf("expected strict=false")
	}
	if len(report.MissingMappings) == 0 {
		t.Fatalf("expected missing mapping")
	}
	if len(report.DuplicateIDs) == 0 {
		t.Fatalf("expected duplicate ID")
	}
}
