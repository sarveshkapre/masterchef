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
	ID          string              `json:"id"`
	ProfileID   string              `json:"profile_id"`
	TargetKind  string              `json:"target_kind"`
	TargetName  string              `json:"target_name"`
	Team        string              `json:"team,omitempty"`
	Environment string              `json:"environment,omitempty"`
	Service     string              `json:"service,omitempty"`
	Status      string              `json:"status"` // pass|fail
	Score       int                 `json:"score"`
	StartedAt   time.Time           `json:"started_at"`
	EndedAt     time.Time           `json:"ended_at"`
	Findings    []ComplianceFinding `json:"findings"`
}

type ComplianceScanInput struct {
	ProfileID   string `json:"profile_id"`
	TargetKind  string `json:"target_kind"`
	TargetName  string `json:"target_name"`
	Team        string `json:"team,omitempty"`
	Environment string `json:"environment,omitempty"`
	Service     string `json:"service,omitempty"`
}

type ComplianceExceptionStatus string

const (
	ComplianceExceptionPending  ComplianceExceptionStatus = "pending"
	ComplianceExceptionApproved ComplianceExceptionStatus = "approved"
	ComplianceExceptionRejected ComplianceExceptionStatus = "rejected"
	ComplianceExceptionExpired  ComplianceExceptionStatus = "expired"
)

type ComplianceExceptionApproval struct {
	Actor     string    `json:"actor"`
	Decision  string    `json:"decision"` // approve|reject
	Comment   string    `json:"comment,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type ComplianceException struct {
	ID          string                        `json:"id"`
	ProfileID   string                        `json:"profile_id"`
	ControlID   string                        `json:"control_id"`
	TargetKind  string                        `json:"target_kind"`
	TargetName  string                        `json:"target_name"`
	Reason      string                        `json:"reason"`
	RequestedBy string                        `json:"requested_by"`
	ExpiresAt   time.Time                     `json:"expires_at"`
	Status      ComplianceExceptionStatus     `json:"status"`
	Approvals   []ComplianceExceptionApproval `json:"approvals,omitempty"`
	CreatedAt   time.Time                     `json:"created_at"`
	UpdatedAt   time.Time                     `json:"updated_at"`
}

type ComplianceExceptionInput struct {
	ProfileID   string `json:"profile_id"`
	ControlID   string `json:"control_id"`
	TargetKind  string `json:"target_kind"`
	TargetName  string `json:"target_name"`
	Reason      string `json:"reason"`
	RequestedBy string `json:"requested_by"`
	ExpiresAt   string `json:"expires_at"` // RFC3339
}

type ComplianceScorecard struct {
	Dimension    string    `json:"dimension"`
	Key          string    `json:"key"`
	ScanCount    int       `json:"scan_count"`
	PassCount    int       `json:"pass_count"`
	FailCount    int       `json:"fail_count"`
	AverageScore int       `json:"average_score"`
	LastScanAt   time.Time `json:"last_scan_at"`
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
	mu              sync.RWMutex
	nextProfileID   int64
	nextScanID      int64
	nextConfigID    int64
	nextExceptionID int64
	profiles        map[string]*ComplianceProfile
	scans           map[string]*ComplianceScan
	continuousRuns  map[string]*ComplianceContinuousConfig
	exceptions      map[string]*ComplianceException
}

func NewComplianceStore() *ComplianceStore {
	return &ComplianceStore{
		profiles:       map[string]*ComplianceProfile{},
		scans:          map[string]*ComplianceScan{},
		continuousRuns: map[string]*ComplianceContinuousConfig{},
		exceptions:     map[string]*ComplianceException{},
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

func (s *ComplianceStore) CreateException(in ComplianceExceptionInput) (ComplianceException, error) {
	profileID := strings.TrimSpace(in.ProfileID)
	controlID := strings.TrimSpace(in.ControlID)
	targetKind := strings.TrimSpace(in.TargetKind)
	targetName := strings.TrimSpace(in.TargetName)
	reason := strings.TrimSpace(in.Reason)
	requestedBy := strings.TrimSpace(in.RequestedBy)
	if profileID == "" || controlID == "" || targetKind == "" || targetName == "" || reason == "" || requestedBy == "" {
		return ComplianceException{}, errors.New("profile_id, control_id, target_kind, target_name, reason, and requested_by are required")
	}
	expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(in.ExpiresAt))
	if err != nil {
		return ComplianceException{}, errors.New("expires_at must be RFC3339")
	}
	now := time.Now().UTC()
	if !expiresAt.After(now) {
		return ComplianceException{}, errors.New("expires_at must be in the future")
	}
	if expiresAt.After(now.Add(365 * 24 * time.Hour)) {
		return ComplianceException{}, errors.New("expires_at must be within 365 days")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.profiles[profileID]
	if !ok {
		return ComplianceException{}, errors.New("compliance profile not found")
	}
	if !profileHasControl(profile, controlID) {
		return ComplianceException{}, errors.New("control_id does not belong to profile")
	}
	s.nextExceptionID++
	item := ComplianceException{
		ID:          "compliance-exception-" + itoa(s.nextExceptionID),
		ProfileID:   profileID,
		ControlID:   controlID,
		TargetKind:  targetKind,
		TargetName:  targetName,
		Reason:      reason,
		RequestedBy: requestedBy,
		ExpiresAt:   expiresAt,
		Status:      ComplianceExceptionPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.exceptions[item.ID] = &item
	return cloneComplianceException(item), nil
}

func (s *ComplianceStore) ListExceptions() []ComplianceException {
	now := time.Now().UTC()
	s.mu.Lock()
	s.expireExceptionsLocked(now)
	out := make([]ComplianceException, 0, len(s.exceptions))
	for _, item := range s.exceptions {
		out = append(out, cloneComplianceException(*item))
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *ComplianceStore) ApproveException(id, actor, comment string) (ComplianceException, error) {
	return s.decideException(id, actor, "approve", comment)
}

func (s *ComplianceStore) RejectException(id, actor, comment string) (ComplianceException, error) {
	return s.decideException(id, actor, "reject", comment)
}

func (s *ComplianceStore) decideException(id, actor, decision, comment string) (ComplianceException, error) {
	id = strings.TrimSpace(id)
	actor = strings.TrimSpace(actor)
	if id == "" || actor == "" {
		return ComplianceException{}, errors.New("id and actor are required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireExceptionsLocked(now)
	item, ok := s.exceptions[id]
	if !ok {
		return ComplianceException{}, errors.New("compliance exception not found")
	}
	if item.Status != ComplianceExceptionPending {
		return ComplianceException{}, errors.New("compliance exception is not pending")
	}
	item.Approvals = append(item.Approvals, ComplianceExceptionApproval{
		Actor:     actor,
		Decision:  decision,
		Comment:   strings.TrimSpace(comment),
		CreatedAt: now,
	})
	if decision == "approve" {
		item.Status = ComplianceExceptionApproved
	} else {
		item.Status = ComplianceExceptionRejected
	}
	item.UpdatedAt = now
	return cloneComplianceException(*item), nil
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
	s.expireExceptionsLocked(time.Now().UTC())
	profile, ok := s.profiles[profileID]
	if !ok {
		return ComplianceScan{}, errors.New("compliance profile not found")
	}
	startedAt := time.Now().UTC()
	findings := make([]ComplianceFinding, 0, len(profile.Controls))
	passCount := 0
	for _, control := range profile.Controls {
		status := evaluateComplianceControl(control.ID, targetKind, targetName)
		if ex, hasEx := s.matchApprovedExceptionLocked(profileID, control.ID, targetKind, targetName, startedAt); hasEx {
			status = "waived"
			findings = append(findings, ComplianceFinding{
				ControlID: control.ID,
				Status:    status,
				Severity:  control.Severity,
				Message:   fmt.Sprintf("%s control waived by approved exception %s", control.ID, ex.ID),
				Evidence:  fmt.Sprintf("exception=%s target=%s/%s", ex.ID, targetKind, targetName),
			})
			passCount++
			continue
		}
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
		ID:          "compliance-scan-" + itoa(s.nextScanID),
		ProfileID:   profile.ID,
		TargetKind:  targetKind,
		TargetName:  targetName,
		Team:        strings.TrimSpace(in.Team),
		Environment: strings.TrimSpace(in.Environment),
		Service:     strings.TrimSpace(in.Service),
		Status:      status,
		Score:       score,
		StartedAt:   startedAt,
		EndedAt:     time.Now().UTC(),
		Findings:    findings,
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

func (s *ComplianceStore) ScorecardsByDimension(dimension string) ([]ComplianceScorecard, error) {
	dimension = strings.ToLower(strings.TrimSpace(dimension))
	if dimension == "" {
		dimension = "team"
	}
	if dimension != "team" && dimension != "environment" && dimension != "service" {
		return nil, errors.New("dimension must be team, environment, or service")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	type agg struct {
		scans      int
		pass       int
		fail       int
		scoreTotal int
		lastScanAt time.Time
	}
	byKey := map[string]*agg{}
	for _, scan := range s.scans {
		key := scorecardDimensionKey(*scan, dimension)
		if key == "" {
			continue
		}
		item := byKey[key]
		if item == nil {
			item = &agg{}
			byKey[key] = item
		}
		item.scans++
		item.scoreTotal += scan.Score
		if scan.Status == "pass" {
			item.pass++
		} else {
			item.fail++
		}
		if scan.EndedAt.After(item.lastScanAt) {
			item.lastScanAt = scan.EndedAt
		}
	}
	out := make([]ComplianceScorecard, 0, len(byKey))
	for key, item := range byKey {
		avg := 0
		if item.scans > 0 {
			avg = item.scoreTotal / item.scans
		}
		out = append(out, ComplianceScorecard{
			Dimension:    dimension,
			Key:          key,
			ScanCount:    item.scans,
			PassCount:    item.pass,
			FailCount:    item.fail,
			AverageScore: avg,
			LastScanAt:   item.lastScanAt,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AverageScore != out[j].AverageScore {
			return out[i].AverageScore > out[j].AverageScore
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
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

func (s *ComplianceStore) matchApprovedExceptionLocked(profileID, controlID, targetKind, targetName string, now time.Time) (ComplianceException, bool) {
	for _, item := range s.exceptions {
		if item.Status != ComplianceExceptionApproved {
			continue
		}
		if !now.Before(item.ExpiresAt) {
			continue
		}
		if item.ProfileID != profileID || item.ControlID != controlID {
			continue
		}
		if item.TargetKind != targetKind || item.TargetName != targetName {
			continue
		}
		return cloneComplianceException(*item), true
	}
	return ComplianceException{}, false
}

func (s *ComplianceStore) expireExceptionsLocked(now time.Time) {
	for _, item := range s.exceptions {
		if (item.Status == ComplianceExceptionApproved || item.Status == ComplianceExceptionPending) && !now.Before(item.ExpiresAt) {
			item.Status = ComplianceExceptionExpired
			item.UpdatedAt = now
		}
	}
}

func profileHasControl(profile *ComplianceProfile, controlID string) bool {
	for _, control := range profile.Controls {
		if control.ID == controlID {
			return true
		}
	}
	return false
}

func scorecardDimensionKey(scan ComplianceScan, dimension string) string {
	switch dimension {
	case "team":
		return strings.TrimSpace(scan.Team)
	case "environment":
		return strings.TrimSpace(scan.Environment)
	case "service":
		return strings.TrimSpace(scan.Service)
	default:
		return ""
	}
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

func cloneComplianceException(in ComplianceException) ComplianceException {
	out := in
	out.Approvals = append([]ComplianceExceptionApproval{}, in.Approvals...)
	return out
}
