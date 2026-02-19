package control

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type MigrationTranslateInput struct {
	SourcePlatform string `json:"source_platform"` // chef|ansible|puppet
	SourceContent  string `json:"source_content"`
	Workload       string `json:"workload,omitempty"`
}

type MigrationTranslation struct {
	ID               string               `json:"id"`
	SourcePlatform   string               `json:"source_platform"`
	Workload         string               `json:"workload,omitempty"`
	SourceDigest     string               `json:"source_digest"`
	DetectedFeatures []string             `json:"detected_features,omitempty"`
	MappedFeatures   []string             `json:"mapped_features,omitempty"`
	UnmappedFeatures []string             `json:"unmapped_features,omitempty"`
	GeneratedConfig  string               `json:"generated_config"`
	DiffReport       []MigrationDiffEntry `json:"diff_report,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
}

type MigrationEquivalenceInput struct {
	TranslationID  string                   `json:"translation_id"`
	SemanticChecks []MigrationSemanticCheck `json:"semantic_checks,omitempty"`
}

type MigrationEquivalenceResult struct {
	TranslationID string                    `json:"translation_id"`
	Pass          bool                      `json:"pass"`
	Score         int                       `json:"score"`
	Failures      int                       `json:"failures"`
	Results       []MigrationSemanticResult `json:"results,omitempty"`
	DiffReport    []MigrationDiffEntry      `json:"diff_report,omitempty"`
	CheckedAt     time.Time                 `json:"checked_at"`
}

type MigrationDiffReportResult struct {
	TranslationID   string               `json:"translation_id"`
	ParityScore     int                  `json:"parity_score"`
	DiffReport      []MigrationDiffEntry `json:"diff_report,omitempty"`
	Recommendations []string             `json:"recommendations,omitempty"`
	GeneratedAt     time.Time            `json:"generated_at"`
}

type MigrationToolingStore struct {
	mu           sync.RWMutex
	nextID       int64
	translations map[string]MigrationTranslation
}

func NewMigrationToolingStore() *MigrationToolingStore {
	return &MigrationToolingStore{
		translations: map[string]MigrationTranslation{},
	}
}

func (s *MigrationToolingStore) Translate(in MigrationTranslateInput) (MigrationTranslation, error) {
	platform := normalizeFeature(in.SourcePlatform)
	content := strings.TrimSpace(in.SourceContent)
	if platform == "" {
		return MigrationTranslation{}, errors.New("source_platform is required")
	}
	if content == "" {
		return MigrationTranslation{}, errors.New("source_content is required")
	}
	if platform != "chef" && platform != "ansible" && platform != "puppet" {
		return MigrationTranslation{}, errors.New("source_platform must be chef, ansible, or puppet")
	}
	detected := detectMigrationToolingFeatures(platform, content)
	mapped, unmapped := mapMigrationToolingFeatures(detected)
	config := buildMigrationToolingConfig(platform, mapped, strings.TrimSpace(in.Workload))
	diff := buildMigrationToolingDiff(mapped, unmapped)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	item := MigrationTranslation{
		ID:               "migration-translate-" + itoa(s.nextID),
		SourcePlatform:   platform,
		Workload:         strings.TrimSpace(in.Workload),
		SourceDigest:     digestString(content),
		DetectedFeatures: detected,
		MappedFeatures:   mapped,
		UnmappedFeatures: unmapped,
		GeneratedConfig:  config,
		DiffReport:       diff,
		CreatedAt:        time.Now().UTC(),
	}
	s.translations[item.ID] = item
	return item, nil
}

func (s *MigrationToolingStore) ListTranslations() []MigrationTranslation {
	s.mu.RLock()
	out := make([]MigrationTranslation, 0, len(s.translations))
	for _, item := range s.translations {
		out = append(out, cloneMigrationTranslation(item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *MigrationToolingStore) GetTranslation(id string) (MigrationTranslation, bool) {
	s.mu.RLock()
	item, ok := s.translations[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return MigrationTranslation{}, false
	}
	return cloneMigrationTranslation(item), true
}

func (s *MigrationToolingStore) Equivalence(in MigrationEquivalenceInput) (MigrationEquivalenceResult, error) {
	translationID := strings.TrimSpace(in.TranslationID)
	if translationID == "" {
		return MigrationEquivalenceResult{}, errors.New("translation_id is required")
	}
	item, ok := s.GetTranslation(translationID)
	if !ok {
		return MigrationEquivalenceResult{}, errors.New("translation not found")
	}
	checks := in.SemanticChecks
	if len(checks) == 0 {
		checks = make([]MigrationSemanticCheck, 0, len(item.DetectedFeatures))
		for _, feature := range item.DetectedFeatures {
			checks = append(checks, MigrationSemanticCheck{
				Name:       "feature-" + strings.ReplaceAll(feature, " ", "-"),
				Expected:   feature,
				Translated: feature,
			})
		}
	}
	results, diff, failures := evaluateSemanticChecks(checks)
	score := 100 - failures*20
	if score < 0 {
		score = 0
	}
	return MigrationEquivalenceResult{
		TranslationID: translationID,
		Pass:          failures == 0,
		Score:         score,
		Failures:      failures,
		Results:       results,
		DiffReport:    diff,
		CheckedAt:     time.Now().UTC(),
	}, nil
}

func (s *MigrationToolingStore) DiffReport(translationID string) (MigrationDiffReportResult, error) {
	translationID = strings.TrimSpace(translationID)
	if translationID == "" {
		return MigrationDiffReportResult{}, errors.New("translation_id is required")
	}
	item, ok := s.GetTranslation(translationID)
	if !ok {
		return MigrationDiffReportResult{}, errors.New("translation not found")
	}
	total := len(item.DetectedFeatures)
	if total == 0 {
		total = 1
	}
	parity := (len(item.MappedFeatures) * 100) / total
	recommendations := buildMigrationRecommendations(item.UnmappedFeatures, 0, 0)
	if len(recommendations) == 0 {
		recommendations = []string{"translation has parity coverage; proceed with staged validation"}
	}
	return MigrationDiffReportResult{
		TranslationID:   translationID,
		ParityScore:     parity,
		DiffReport:      append([]MigrationDiffEntry{}, item.DiffReport...),
		Recommendations: recommendations,
		GeneratedAt:     time.Now().UTC(),
	}, nil
}

func detectMigrationToolingFeatures(platform, content string) []string {
	contentLower := strings.ToLower(content)
	patterns := migrationToolingPatterns(platform)
	out := make([]string, 0, len(patterns))
	for feature, keys := range patterns {
		for _, key := range keys {
			if strings.Contains(contentLower, key) {
				out = append(out, feature)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func migrationToolingPatterns(platform string) map[string][]string {
	switch platform {
	case "chef":
		return map[string][]string{
			"recipes":      {"recipe", "cookbook"},
			"resources":    {"package", "service", "template", "execute", "file"},
			"roles":        {"role"},
			"environments": {"environment"},
			"data bags":    {"data_bag", "data bag"},
			"handlers":     {"handler", "report_handler"},
		}
	case "ansible":
		return map[string][]string{
			"playbooks":         {"hosts:", "tasks:", "playbook"},
			"roles":             {"roles:", "include_role"},
			"handlers":          {"handlers:", "notify:"},
			"become":            {"become:", "become_user:"},
			"delegate to":       {"delegate_to:"},
			"dynamic inventory": {"plugin:", "aws_ec2", "constructed"},
			"vault":             {"ansible-vault", "!vault"},
		}
	case "puppet":
		return map[string][]string{
			"resource dsl":        {"package {", "service {", "file {"},
			"catalog compilation": {"catalog", "compile"},
			"hiera":               {"hiera", "lookup("},
			"facter":              {"facter", "$facts"},
			"enc":                 {"node_classifier", "enc"},
			"modules":             {"class ", "define "},
			"orchestrator":        {"plan ", "task "},
		}
	default:
		return map[string][]string{}
	}
}

var migrationToolingFeatureToResource = map[string]string{
	"recipes":             "command",
	"resources":           "package",
	"roles":               "user",
	"environments":        "template",
	"data bags":           "template",
	"handlers":            "service",
	"playbooks":           "command",
	"become":              "command",
	"delegate to":         "command",
	"dynamic inventory":   "template",
	"vault":               "template",
	"resource dsl":        "file",
	"catalog compilation": "command",
	"hiera":               "template",
	"facter":              "command",
	"enc":                 "template",
	"modules":             "template",
	"orchestrator":        "command",
}

func mapMigrationToolingFeatures(detected []string) ([]string, []string) {
	mapped := make([]string, 0, len(detected))
	unmapped := make([]string, 0)
	for _, feature := range detected {
		if _, ok := migrationToolingFeatureToResource[feature]; ok {
			mapped = append(mapped, feature)
			continue
		}
		unmapped = append(unmapped, feature)
	}
	return mapped, unmapped
}

func buildMigrationToolingConfig(platform string, features []string, workload string) string {
	var b strings.Builder
	b.WriteString("version: v1\n")
	b.WriteString("metadata:\n")
	b.WriteString("  source_platform: " + platform + "\n")
	if workload != "" {
		b.WriteString("  workload: " + workload + "\n")
	}
	b.WriteString("resources:\n")
	if len(features) == 0 {
		b.WriteString("  - id: migrated-placeholder\n")
		b.WriteString("    type: command\n")
		b.WriteString("    host: localhost\n")
		b.WriteString("    command: echo \"no mappable features detected\"\n")
		return b.String()
	}
	for i, feature := range features {
		id := "migrated-" + strings.ReplaceAll(feature, " ", "-") + "-" + itoa(int64(i+1))
		resType := migrationToolingFeatureToResource[feature]
		b.WriteString("  - id: " + id + "\n")
		b.WriteString("    type: " + resType + "\n")
		b.WriteString("    host: localhost\n")
		b.WriteString("    tags: [migration]\n")
		b.WriteString("    metadata:\n")
		b.WriteString("      mc_feature: \"" + feature + "\"\n")
		switch resType {
		case "package":
			b.WriteString("    name: migrated-package\n")
			b.WriteString("    state: present\n")
		case "service":
			b.WriteString("    name: migrated-service\n")
			b.WriteString("    state: started\n")
		case "file":
			b.WriteString("    path: /tmp/migrated-file\n")
			b.WriteString("    content: \"migrated\"\n")
		case "template":
			b.WriteString("    path: /tmp/migrated-template\n")
			b.WriteString("    content: \"source feature: " + feature + "\"\n")
		default:
			b.WriteString("    command: echo \"migrated feature " + feature + "\"\n")
		}
	}
	return b.String()
}

func buildMigrationToolingDiff(mapped, unmapped []string) []MigrationDiffEntry {
	out := make([]MigrationDiffEntry, 0, len(mapped)+len(unmapped))
	for _, feature := range mapped {
		out = append(out, MigrationDiffEntry{
			Feature:         feature,
			Severity:        "info",
			Message:         "feature translated into Masterchef resource model",
			SuggestedAction: "validate generated resource behavior in check/noop mode",
		})
	}
	for _, feature := range unmapped {
		out = append(out, MigrationDiffEntry{
			Feature:         feature,
			Severity:        "error",
			Message:         "feature has no direct translation mapping",
			SuggestedAction: "add compatibility shim or custom provider before rollout",
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Feature < out[j].Feature })
	return out
}

func digestString(in string) string {
	hash := sha256.Sum256([]byte(in))
	return "sha256:" + hex.EncodeToString(hash[:])
}

func cloneMigrationTranslation(in MigrationTranslation) MigrationTranslation {
	out := in
	out.DetectedFeatures = append([]string{}, in.DetectedFeatures...)
	out.MappedFeatures = append([]string{}, in.MappedFeatures...)
	out.UnmappedFeatures = append([]string{}, in.UnmappedFeatures...)
	out.DiffReport = append([]MigrationDiffEntry{}, in.DiffReport...)
	return out
}
