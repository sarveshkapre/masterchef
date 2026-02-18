package features

import "testing"

func TestParse_FeaturesAndMatrix(t *testing.T) {
	doc, err := Parse("../../features.md")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(doc.Bullets) == 0 {
		t.Fatalf("expected bullet features")
	}
	if len(doc.Matrix) == 0 {
		t.Fatalf("expected matrix rows")
	}

	var (
		chef, ansible, puppet, salt int
	)
	for _, row := range doc.Matrix {
		switch row.Competitor {
		case "Chef":
			chef++
		case "Ansible":
			ansible++
		case "Puppet":
			puppet++
		case "Salt":
			salt++
		}
	}
	if chef == 0 || ansible == 0 || puppet == 0 || salt == 0 {
		t.Fatalf("expected all competitor sections in matrix, got chef=%d ansible=%d puppet=%d salt=%d",
			chef, ansible, puppet, salt)
	}
}
