package control

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type ComplianceControl struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Severity    string `json:"severity"` // low|medium|high|critical
}

type ComplianceProfile struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Framework string              `json:"framework"` // cis|stig|custom
	Version   string              `json:"version,omitempty"`
	Controls  []ComplianceControl `json:"controls"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

type ComplianceProfileInput struct {
	Name      string              `json:"name"`
	Framework string              `json:"framework"`
	Version   string              `json:"version,omitempty"`
	Controls  []ComplianceControl `json:"controls"`
}

type ComplianceFinding struct {
	ControlID string `json:"control_id"`
	Status    string `json:"status"` // pass|fail
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	Evidence  string `json:"evidence"`
}

type ComplianceScan struct {
	ID         string              `json:"id"`
	ProfileID  string              `json:"profile_id"`
	TargetKind string              `json:"target_kind"`
	TargetName string              `json:"target_name"`
	Status     string              `json:"status"` // pass|fail
	Score      int                 `json:"score"`
	StartedAt  time.Time           `json:"started_at"`
	EndedAt    time.Time           `json:"ended_at"`
	Findings   []ComplianceFinding `json:"findings"`
}

type ComplianceScanInput struct {
	ProfileID  string `json:"profile_id"`
	TargetKind string `json:"target_kind"`
	TargetName string `json:"target_name"`
}

type ComplianceContinuousConfig struct {
	ID              string     `json:"id"`
	ProfileID       string     `json:"profile_id"`
	TargetKind      string     `json:"target_kind"`
	TargetName      string     `json:"target_name"`
	IntervalSeconds int        `json:"interval_seconds"`
	Enabled         bool       `json:"enabled"`
	LastScanID      string     `json:"last_scan_id,omitempty"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type ComplianceContinuousInput struct {
	ProfileID       string `json:"profile_id"`
	TargetKind      string `json:"target_kind"`
	TargetName      string `json:"target_name"`
	IntervalSeconds int    `json:"interval_seconds,omitempty"`
	Enabled         *bool  `json:"enabled,omitempty"`
}

type ComplianceStore struct {
	mu             sync.RWMutex
	nextProfileID  int64
	nextScanID     int64
	nextConfigID   int64
	profiles       map[string]*ComplianceProfile
	scans          map[string]*ComplianceScan
	continuousRuns map[string]*ComplianceContinuousConfig
}

func NewComplianceStore() *ComplianceStore {
	return &ComplianceStore{
		profiles:       map[string]*ComplianceProfile{},
		scans:          map[string]*ComplianceScan{},
		continuousRuns: map[string]*ComplianceContinuousConfig{},
	}
}

func (s *ComplianceStore) CreateProfile(in ComplianceProfileInput) (ComplianceProfile, error) {
	name := strings.TrimSpace(in.Name)
	framework := strings.ToLower(strings.TrimSpace(in.Framework))
	if name == "" || framework == "" {
		return ComplianceProfile{}, errors.New("name and framework are required")
	}
	if framework != "cis" && framework != "stig" && framework != "custom" {
		return ComplianceProfile{}, errors.New("framework must be cis, stig, or custom")
	}
	controls, err := normalizeComplianceControls(in.Controls)
	if err != nil {
		return ComplianceProfile{}, err
	}
	now := time.Now().UTC()
	item := ComplianceProfile{
		Name:      name,
		Framework: framework,
		Version:   strings.TrimSpace(in.Version),
		Controls:  controls,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextProfileID++
	item.ID = "compliance-profile-" + itoa(s.nextProfileID)
	s.profiles[item.ID] = &item
	return cloneComplianceProfile(item), nil
}

func (s *ComplianceStore) ListProfiles() []ComplianceProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ComplianceProfile, 0, len(s.profiles))
	for _, item := range s.profiles {
		out = append(out, cloneComplianceProfile(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *ComplianceStore) GetProfile(id string) (ComplianceProfile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.profiles[strings.TrimSpace(id)]
	if !ok {
		return ComplianceProfile{}, false
	}
	return cloneComplianceProfile(*item), true
}

func (s *ComplianceStore) RunScan(in ComplianceScanInput) (ComplianceScan, error) {
	profileID := strings.TrimSpace(in.ProfileID)
	targetKind := strings.TrimSpace(in.TargetKind)
	targetName := strings.TrimSpace(in.TargetName)
	if profileID == "" || targetKind == "" || targetName == "" {
		return ComplianceScan{}, errors.New("profile_id, target_kind, and target_name are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.profiles[profileID]
	if !ok {
		return ComplianceScan{}, errors.New("compliance profile not found")
	}
	startedAt := time.Now().UTC()
	findings := make([]ComplianceFinding, 0, len(profile.Controls))
	passCount := 0
	for _, control := range profile.Controls {
		status := evaluateComplianceControl(control.ID, targetKind, targetName)
		if status == "pass" {
			passCount++
		}
		findings = append(findings, ComplianceFinding{
			ControlID: control.ID,
			Status:    status,
			Severity:  control.Severity,
			Message:   fmt.Sprintf("%s control evaluated as %s", control.ID, status),
			Evidence:  fmt.Sprintf("target=%s/%s framework=%s", targetKind, targetName, profile.Framework),
		})
	}
	score := 100
	if len(findings) > 0 {
		score = (passCount * 100) / len(findings)
	}
	status := "pass"
	if score < 100 {
		status = "fail"
	}
	s.nextScanID++
	scan := ComplianceScan{
		ID:         "compliance-scan-" + itoa(s.nextScanID),
		ProfileID:  profile.ID,
		TargetKind: targetKind,
		TargetName: targetName,
		Status:     status,
		Score:      score,
		StartedAt:  startedAt,
		EndedAt:    time.Now().UTC(),
		Findings:   findings,
	}
	s.scans[scan.ID] = &scan
	return cloneComplianceScan(scan), nil
}

func (s *ComplianceStore) ListScans() []ComplianceScan {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ComplianceScan, 0, len(s.scans))
	for _, item := range s.scans {
		out = append(out, cloneComplianceScan(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

func (s *ComplianceStore) GetScan(id string) (ComplianceScan, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.scans[strings.TrimSpace(id)]
	if !ok {
		return ComplianceScan{}, false
	}
	return cloneComplianceScan(*item), true
}

func (s *ComplianceStore) UpsertContinuousConfig(in ComplianceContinuousInput) (ComplianceContinuousConfig, error) {
	profileID := strings.TrimSpace(in.ProfileID)
	targetKind := strings.TrimSpace(in.TargetKind)
	targetName := strings.TrimSpace(in.TargetName)
	if profileID == "" || targetKind == "" || targetName == "" {
		return ComplianceContinuousConfig{}, errors.New("profile_id, target_kind, and target_name are required")
	}
	interval := in.IntervalSeconds
	if interval <= 0 {
		interval = 300
	}
	if interval < 60 {
		return ComplianceContinuousConfig{}, errors.New("interval_seconds must be >= 60")
	}
	if interval > 86400 {
		return ComplianceContinuousConfig{}, errors.New("interval_seconds must be <= 86400")
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.profiles[profileID]; !ok {
		return ComplianceContinuousConfig{}, errors.New("compliance profile not found")
	}
	for _, item := range s.continuousRuns {
		if item.ProfileID == profileID && item.TargetKind == targetKind && item.TargetName == targetName {
			item.IntervalSeconds = interval
			item.Enabled = enabled
			item.UpdatedAt = time.Now().UTC()
			return cloneComplianceContinuousConfig(*item), nil
		}
	}
	now := time.Now().UTC()
	s.nextConfigID++
	item := ComplianceContinuousConfig{
		ID:              "compliance-continuous-" + itoa(s.nextConfigID),
		ProfileID:       profileID,
		TargetKind:      targetKind,
		TargetName:      targetName,
		IntervalSeconds: interval,
		Enabled:         enabled,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	s.continuousRuns[item.ID] = &item
	return cloneComplianceContinuousConfig(item), nil
}

func (s *ComplianceStore) ListContinuousConfigs() []ComplianceContinuousConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ComplianceContinuousConfig, 0, len(s.continuousRuns))
	for _, item := range s.continuousRuns {
		out = append(out, cloneComplianceContinuousConfig(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *ComplianceStore) RunContinuousScan(configID string) (ComplianceScan, ComplianceContinuousConfig, error) {
	configID = strings.TrimSpace(configID)
	if configID == "" {
		return ComplianceScan{}, ComplianceContinuousConfig{}, errors.New("continuous config id is required")
	}
	s.mu.Lock()
	config, ok := s.continuousRuns[configID]
	if !ok {
		s.mu.Unlock()
		return ComplianceScan{}, ComplianceContinuousConfig{}, errors.New("continuous compliance config not found")
	}
	if !config.Enabled {
		s.mu.Unlock()
		return ComplianceScan{}, ComplianceContinuousConfig{}, errors.New("continuous compliance config is disabled")
	}
	profileID := config.ProfileID
	targetKind := config.TargetKind
	targetName := config.TargetName
	s.mu.Unlock()

	scan, err := s.RunScan(ComplianceScanInput{
		ProfileID:  profileID,
		TargetKind: targetKind,
		TargetName: targetName,
	})
	if err != nil {
		return ComplianceScan{}, ComplianceContinuousConfig{}, err
	}

	s.mu.Lock()
	config = s.continuousRuns[configID]
	now := time.Now().UTC()
	config.LastScanID = scan.ID
	config.LastRunAt = &now
	config.UpdatedAt = now
	updated := cloneComplianceContinuousConfig(*config)
	s.mu.Unlock()
	return scan, updated, nil
}

func (s *ComplianceStore) ExportEvidence(scanID, format string) ([]byte, string, error) {
	scan, ok := s.GetScan(scanID)
	if !ok {
		return nil, "", errors.New("compliance scan not found")
	}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		payload := map[string]any{
			"scan":      scan,
			"generated": time.Now().UTC(),
			"format":    "json",
		}
		out, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return nil, "", err
		}
		return out, "application/json", nil
	case "csv":
		builder := &strings.Builder{}
		w := csv.NewWriter(builder)
		_ = w.Write([]string{"scan_id", "profile_id", "target_kind", "target_name", "control_id", "status", "severity", "message", "evidence"})
		for _, finding := range scan.Findings {
			_ = w.Write([]string{
				scan.ID,
				scan.ProfileID,
				scan.TargetKind,
				scan.TargetName,
				finding.ControlID,
				finding.Status,
				finding.Severity,
				finding.Message,
				finding.Evidence,
			})
		}
		w.Flush()
		return []byte(builder.String()), "text/csv", nil
	case "sarif":
		sarif := map[string]any{
			"version": "2.1.0",
			"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
			"runs": []any{
				map[string]any{
					"tool": map[string]any{
						"driver": map[string]any{
							"name":    "masterchef-compliance",
							"version": "v1",
						},
					},
					"results": sarifResults(scan),
				},
			},
		}
		out, err := json.MarshalIndent(sarif, "", "  ")
		if err != nil {
			return nil, "", err
		}
		return out, "application/sarif+json", nil
	default:
		return nil, "", errors.New("unsupported evidence format")
	}
}

func sarifResults(scan ComplianceScan) []any {
	out := make([]any, 0, len(scan.Findings))
	for _, finding := range scan.Findings {
		level := "note"
		if finding.Status == "fail" {
			level = "error"
		}
		out = append(out, map[string]any{
			"ruleId": finding.ControlID,
			"level":  level,
			"message": map[string]any{
				"text": finding.Message,
			},
			"properties": map[string]any{
				"severity": finding.Severity,
				"status":   finding.Status,
				"evidence": finding.Evidence,
				"target":   scan.TargetKind + "/" + scan.TargetName,
			},
		})
	}
	return out
}

func evaluateComplianceControl(controlID, targetKind, targetName string) string {
	key := controlID + "|" + targetKind + "|" + targetName
	sum := sha256.Sum256([]byte(key))
	if int(sum[0])%5 == 0 {
		return "fail"
	}
	return "pass"
}

func normalizeComplianceControls(in []ComplianceControl) ([]ComplianceControl, error) {
	if len(in) == 0 {
		return nil, errors.New("at least one control is required")
	}
	out := make([]ComplianceControl, 0, len(in))
	for _, control := range in {
		id := strings.TrimSpace(control.ID)
		desc := strings.TrimSpace(control.Description)
		sev := strings.ToLower(strings.TrimSpace(control.Severity))
		if id == "" || desc == "" {
			return nil, errors.New("control id and description are required")
		}
		if sev == "" {
			sev = "medium"
		}
		switch sev {
		case "low", "medium", "high", "critical":
		default:
			return nil, errors.New("control severity must be low, medium, high, or critical")
		}
		out = append(out, ComplianceControl{
			ID:          id,
			Description: desc,
			Severity:    sev,
		})
	}
	return out, nil
}

func cloneComplianceProfile(in ComplianceProfile) ComplianceProfile {
	out := in
	out.Controls = append([]ComplianceControl{}, in.Controls...)
	return out
}

func cloneComplianceScan(in ComplianceScan) ComplianceScan {
	out := in
	out.Findings = append([]ComplianceFinding{}, in.Findings...)
	return out
}

func cloneComplianceContinuousConfig(in ComplianceContinuousConfig) ComplianceContinuousConfig {
	out := in
	if in.LastRunAt != nil {
		lastRun := *in.LastRunAt
		out.LastRunAt = &lastRun
	}
	return out
}
