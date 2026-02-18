package features

import (
	"fmt"
	"strings"
)

type VerifyReport struct {
	TotalRows            int      `json:"total_rows"`
	UniqueIDs            int      `json:"unique_ids"`
	MissingMappings      []string `json:"missing_mappings"`
	DuplicateIDs         []string `json:"duplicate_ids"`
	MissingCompetitors   []string `json:"missing_competitors"`
	CompetitorRowCount   map[string]int
	BulletFeatureCount   int `json:"bullet_feature_count"`
	TraceabilityIsStrict bool
}

func Verify(doc *Document) VerifyReport {
	report := VerifyReport{
		TotalRows:          len(doc.Matrix),
		CompetitorRowCount: map[string]int{},
		BulletFeatureCount: len(doc.Bullets),
	}

	bulletsSet := map[string]struct{}{}
	for _, b := range doc.Bullets {
		bulletsSet[normalize(b)] = struct{}{}
	}

	idSet := map[string]struct{}{}
	for _, row := range doc.Matrix {
		report.CompetitorRowCount[row.Competitor]++

		if _, ok := idSet[row.ID]; ok {
			report.DuplicateIDs = append(report.DuplicateIDs, row.ID)
		}
		idSet[row.ID] = struct{}{}

		if _, ok := bulletsSet[normalize(row.Mapping)]; !ok {
			report.MissingMappings = append(report.MissingMappings, row.ID+": "+row.Mapping)
		}
	}

	report.UniqueIDs = len(idSet)
	for _, c := range []string{"Chef", "Ansible", "Puppet", "Salt"} {
		if report.CompetitorRowCount[c] == 0 {
			report.MissingCompetitors = append(report.MissingCompetitors, c)
		}
	}

	report.TraceabilityIsStrict =
		len(report.MissingMappings) == 0 &&
			len(report.DuplicateIDs) == 0 &&
			len(report.MissingCompetitors) == 0
	return report
}

func normalize(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

func (r VerifyReport) Error() error {
	if r.TraceabilityIsStrict {
		return nil
	}
	return fmt.Errorf("traceability verification failed: missing_mappings=%d duplicate_ids=%d missing_competitors=%d",
		len(r.MissingMappings), len(r.DuplicateIDs), len(r.MissingCompetitors))
}
