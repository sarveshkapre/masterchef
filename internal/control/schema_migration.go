package control

import (
	"errors"
	"sync"
	"time"
)

type SchemaCompatibilityReport struct {
	FromVersion           int    `json:"from_version"`
	ToVersion             int    `json:"to_version"`
	ForwardCompatible     bool   `json:"forward_compatible"`
	BackwardCompatible    bool   `json:"backward_compatible"`
	RequiresMigrationPlan bool   `json:"requires_migration_plan"`
	Reason                string `json:"reason,omitempty"`
}

type SchemaMigrationRecord struct {
	ID            string                    `json:"id"`
	FromVersion   int                       `json:"from_version"`
	ToVersion     int                       `json:"to_version"`
	PlanRef       string                    `json:"plan_ref"`
	Notes         string                    `json:"notes,omitempty"`
	AppliedAt     time.Time                 `json:"applied_at"`
	Compatibility SchemaCompatibilityReport `json:"compatibility"`
}

type SchemaMigrationStatus struct {
	CurrentVersion int                     `json:"current_version"`
	History        []SchemaMigrationRecord `json:"history"`
}

type SchemaMigrationManager struct {
	mu             sync.RWMutex
	currentVersion int
	nextID         int64
	history        []SchemaMigrationRecord
}

func NewSchemaMigrationManager(initialVersion int) *SchemaMigrationManager {
	if initialVersion <= 0 {
		initialVersion = 1
	}
	return &SchemaMigrationManager{currentVersion: initialVersion}
}

func (m *SchemaMigrationManager) Check(fromVersion, toVersion int) SchemaCompatibilityReport {
	report := SchemaCompatibilityReport{
		FromVersion:           fromVersion,
		ToVersion:             toVersion,
		RequiresMigrationPlan: fromVersion != toVersion,
	}
	if fromVersion <= 0 || toVersion <= 0 {
		report.Reason = "schema versions must be positive integers"
		return report
	}
	if fromVersion == toVersion {
		report.ForwardCompatible = true
		report.BackwardCompatible = true
		return report
	}
	delta := toVersion - fromVersion
	switch {
	case delta == 1:
		report.ForwardCompatible = true
	case delta == -1:
		report.BackwardCompatible = true
	case delta > 1:
		report.Reason = "forward migration exceeds one version; apply intermediate migration steps"
	default:
		report.Reason = "rollback exceeds one version; apply intermediate rollback steps"
	}
	return report
}

func (m *SchemaMigrationManager) Apply(fromVersion, toVersion int, planRef, notes string) (SchemaMigrationRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if fromVersion != m.currentVersion {
		return SchemaMigrationRecord{}, errors.New("from_version must match current schema version")
	}
	if fromVersion == toVersion {
		return SchemaMigrationRecord{}, errors.New("no schema change requested")
	}
	if planRef == "" {
		return SchemaMigrationRecord{}, errors.New("plan_ref is required for schema version changes")
	}

	report := m.Check(fromVersion, toVersion)
	if toVersion > fromVersion && !report.ForwardCompatible {
		return SchemaMigrationRecord{}, errors.New(report.Reason)
	}
	if toVersion < fromVersion && !report.BackwardCompatible {
		return SchemaMigrationRecord{}, errors.New(report.Reason)
	}

	m.nextID++
	rec := SchemaMigrationRecord{
		ID:            "schema-migration-" + itoa(m.nextID),
		FromVersion:   fromVersion,
		ToVersion:     toVersion,
		PlanRef:       planRef,
		Notes:         notes,
		AppliedAt:     time.Now().UTC(),
		Compatibility: report,
	}
	m.currentVersion = toVersion
	m.history = append(m.history, rec)
	return rec, nil
}

func (m *SchemaMigrationManager) Status() SchemaMigrationStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	history := make([]SchemaMigrationRecord, len(m.history))
	copy(history, m.history)
	return SchemaMigrationStatus{
		CurrentVersion: m.currentVersion,
		History:        history,
	}
}
