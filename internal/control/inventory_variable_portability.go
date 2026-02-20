package control

import (
	"errors"
	"strings"
	"time"
)

type InventoryVariableExportRequest struct {
	Status                        string `json:"status,omitempty"`
	IncludeInventory              bool   `json:"include_inventory,omitempty"`
	IncludeRoles                  bool   `json:"include_roles,omitempty"`
	IncludeEnvironments           bool   `json:"include_environments,omitempty"`
	IncludeEncryptedVariableFiles bool   `json:"include_encrypted_variable_files,omitempty"`
	IncludeNodeHistory            bool   `json:"include_node_history,omitempty"`
}

type InventoryVariableBundle struct {
	Version                string                  `json:"version"`
	ExportedAt             time.Time               `json:"exported_at"`
	Inventory              []ManagedNode           `json:"inventory,omitempty"`
	Roles                  []RoleDefinition        `json:"roles,omitempty"`
	Environments           []EnvironmentDefinition `json:"environments,omitempty"`
	EncryptedVariableFiles []EncryptedVariableFile `json:"encrypted_variable_files,omitempty"`
}

type InventoryVariableImportRequest struct {
	DryRun bool                    `json:"dry_run,omitempty"`
	Bundle InventoryVariableBundle `json:"bundle"`
}

type InventoryVariableImportItemResult struct {
	Kind   string `json:"kind"` // inventory|role|environment|encrypted_variable_file
	Name   string `json:"name,omitempty"`
	Status string `json:"status"` // imported|updated|would_import|would_update|failed
	Error  string `json:"error,omitempty"`
}

type InventoryVariableImportSectionResult struct {
	Imported int                                 `json:"imported"`
	Updated  int                                 `json:"updated"`
	Failed   int                                 `json:"failed"`
	Results  []InventoryVariableImportItemResult `json:"results,omitempty"`
}

type InventoryVariableImportResult struct {
	DryRun                 bool                                 `json:"dry_run"`
	Inventory              InventoryVariableImportSectionResult `json:"inventory"`
	Roles                  InventoryVariableImportSectionResult `json:"roles"`
	Environments           InventoryVariableImportSectionResult `json:"environments"`
	EncryptedVariableFiles InventoryVariableImportSectionResult `json:"encrypted_variable_files"`
}

func BuildInventoryVariableBundle(
	nodes *NodeLifecycleStore,
	roleEnv *RoleEnvironmentStore,
	encryptedVars *EncryptedVariableStore,
	req InventoryVariableExportRequest,
) (InventoryVariableBundle, error) {
	if nodes == nil {
		return InventoryVariableBundle{}, errors.New("node lifecycle store is required")
	}
	if roleEnv == nil {
		return InventoryVariableBundle{}, errors.New("role/environment store is required")
	}
	if encryptedVars == nil {
		return InventoryVariableBundle{}, errors.New("encrypted variable store is required")
	}
	req = normalizeInventoryVariableExportRequest(req)
	if err := validateNodeStatusFilter(req.Status); err != nil {
		return InventoryVariableBundle{}, err
	}

	out := InventoryVariableBundle{
		Version:    "v1",
		ExportedAt: time.Now().UTC(),
	}
	if req.IncludeInventory {
		nodesOut := nodes.List(req.Status)
		if !req.IncludeNodeHistory {
			for i := range nodesOut {
				nodesOut[i].History = nil
			}
		}
		out.Inventory = nodesOut
	}
	if req.IncludeRoles {
		out.Roles = roleEnv.ListRoles()
	}
	if req.IncludeEnvironments {
		out.Environments = roleEnv.ListEnvironments()
	}
	if req.IncludeEncryptedVariableFiles {
		out.EncryptedVariableFiles = encryptedVars.ExportFiles()
	}
	return out, nil
}

func ImportInventoryVariableBundle(
	nodes *NodeLifecycleStore,
	roleEnv *RoleEnvironmentStore,
	encryptedVars *EncryptedVariableStore,
	req InventoryVariableImportRequest,
) (InventoryVariableImportResult, error) {
	if nodes == nil {
		return InventoryVariableImportResult{}, errors.New("node lifecycle store is required")
	}
	if roleEnv == nil {
		return InventoryVariableImportResult{}, errors.New("role/environment store is required")
	}
	if encryptedVars == nil {
		return InventoryVariableImportResult{}, errors.New("encrypted variable store is required")
	}
	if bundleIsEmpty(req.Bundle) {
		return InventoryVariableImportResult{}, errors.New("bundle must include at least one inventory or variable section")
	}

	result := InventoryVariableImportResult{DryRun: req.DryRun}
	importInventorySection(nodes, req.Bundle.Inventory, req.DryRun, &result.Inventory)
	importRoleSection(roleEnv, req.Bundle.Roles, req.DryRun, &result.Roles)
	importEnvironmentSection(roleEnv, req.Bundle.Environments, req.DryRun, &result.Environments)
	importEncryptedVariableSection(encryptedVars, req.Bundle.EncryptedVariableFiles, req.DryRun, &result.EncryptedVariableFiles)
	return result, nil
}

func normalizeInventoryVariableExportRequest(req InventoryVariableExportRequest) InventoryVariableExportRequest {
	if !req.IncludeInventory && !req.IncludeRoles && !req.IncludeEnvironments && !req.IncludeEncryptedVariableFiles {
		req.IncludeInventory = true
		req.IncludeRoles = true
		req.IncludeEnvironments = true
		req.IncludeEncryptedVariableFiles = true
	}
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	return req
}

func bundleIsEmpty(bundle InventoryVariableBundle) bool {
	return len(bundle.Inventory) == 0 &&
		len(bundle.Roles) == 0 &&
		len(bundle.Environments) == 0 &&
		len(bundle.EncryptedVariableFiles) == 0
}

func importInventorySection(
	nodes *NodeLifecycleStore,
	items []ManagedNode,
	dryRun bool,
	out *InventoryVariableImportSectionResult,
) {
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			out.Failed++
			out.Results = append(out.Results, InventoryVariableImportItemResult{
				Kind:   "inventory",
				Status: "failed",
				Error:  "name is required",
			})
			continue
		}
		_, exists := nodes.Get(name)
		if dryRun {
			status := "would_import"
			if exists {
				status = "would_update"
				out.Updated++
			} else {
				out.Imported++
			}
			out.Results = append(out.Results, InventoryVariableImportItemResult{
				Kind:   "inventory",
				Name:   name,
				Status: status,
			})
			continue
		}

		_, created, err := nodes.Enroll(NodeEnrollInput{
			Name:      name,
			Address:   item.Address,
			Transport: item.Transport,
			Labels:    item.Labels,
			Roles:     item.Roles,
			Topology:  item.Topology,
			Source:    item.Source,
		})
		if err != nil {
			out.Failed++
			out.Results = append(out.Results, InventoryVariableImportItemResult{
				Kind:   "inventory",
				Name:   name,
				Status: "failed",
				Error:  err.Error(),
			})
			continue
		}
		if desired := strings.TrimSpace(item.Status); desired != "" && desired != NodeStatusBootstrap {
			if _, err := nodes.SetStatus(name, desired, "imported from inventory bundle"); err != nil {
				out.Failed++
				out.Results = append(out.Results, InventoryVariableImportItemResult{
					Kind:   "inventory",
					Name:   name,
					Status: "failed",
					Error:  err.Error(),
				})
				continue
			}
		}
		status := "updated"
		if created {
			status = "imported"
			out.Imported++
		} else {
			out.Updated++
		}
		out.Results = append(out.Results, InventoryVariableImportItemResult{
			Kind:   "inventory",
			Name:   name,
			Status: status,
		})
	}
}

func importRoleSection(
	roleEnv *RoleEnvironmentStore,
	items []RoleDefinition,
	dryRun bool,
	out *InventoryVariableImportSectionResult,
) {
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			out.Failed++
			out.Results = append(out.Results, InventoryVariableImportItemResult{
				Kind:   "role",
				Status: "failed",
				Error:  "name is required",
			})
			continue
		}
		_, existsErr := roleEnv.GetRole(name)
		exists := existsErr == nil
		if dryRun {
			status := "would_import"
			if exists {
				status = "would_update"
				out.Updated++
			} else {
				out.Imported++
			}
			out.Results = append(out.Results, InventoryVariableImportItemResult{
				Kind:   "role",
				Name:   name,
				Status: status,
			})
			continue
		}
		if _, err := roleEnv.UpsertRole(item); err != nil {
			out.Failed++
			out.Results = append(out.Results, InventoryVariableImportItemResult{
				Kind:   "role",
				Name:   name,
				Status: "failed",
				Error:  err.Error(),
			})
			continue
		}
		status := "updated"
		if !exists {
			status = "imported"
			out.Imported++
		} else {
			out.Updated++
		}
		out.Results = append(out.Results, InventoryVariableImportItemResult{
			Kind:   "role",
			Name:   name,
			Status: status,
		})
	}
}

func importEnvironmentSection(
	roleEnv *RoleEnvironmentStore,
	items []EnvironmentDefinition,
	dryRun bool,
	out *InventoryVariableImportSectionResult,
) {
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			out.Failed++
			out.Results = append(out.Results, InventoryVariableImportItemResult{
				Kind:   "environment",
				Status: "failed",
				Error:  "name is required",
			})
			continue
		}
		_, existsErr := roleEnv.GetEnvironment(name)
		exists := existsErr == nil
		if dryRun {
			status := "would_import"
			if exists {
				status = "would_update"
				out.Updated++
			} else {
				out.Imported++
			}
			out.Results = append(out.Results, InventoryVariableImportItemResult{
				Kind:   "environment",
				Name:   name,
				Status: status,
			})
			continue
		}
		if _, err := roleEnv.UpsertEnvironment(item); err != nil {
			out.Failed++
			out.Results = append(out.Results, InventoryVariableImportItemResult{
				Kind:   "environment",
				Name:   name,
				Status: "failed",
				Error:  err.Error(),
			})
			continue
		}
		status := "updated"
		if !exists {
			status = "imported"
			out.Imported++
		} else {
			out.Updated++
		}
		out.Results = append(out.Results, InventoryVariableImportItemResult{
			Kind:   "environment",
			Name:   name,
			Status: status,
		})
	}
}

func importEncryptedVariableSection(
	encryptedVars *EncryptedVariableStore,
	items []EncryptedVariableFile,
	dryRun bool,
	out *InventoryVariableImportSectionResult,
) {
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			out.Failed++
			out.Results = append(out.Results, InventoryVariableImportItemResult{
				Kind:   "encrypted_variable_file",
				Status: "failed",
				Error:  "name is required",
			})
			continue
		}
		exists := encryptedVars.Exists(name)
		if dryRun {
			status := "would_import"
			if exists {
				status = "would_update"
				out.Updated++
			} else {
				out.Imported++
			}
			out.Results = append(out.Results, InventoryVariableImportItemResult{
				Kind:   "encrypted_variable_file",
				Name:   name,
				Status: status,
			})
			continue
		}

		_, created, err := encryptedVars.UpsertEncryptedFile(item)
		if err != nil {
			out.Failed++
			out.Results = append(out.Results, InventoryVariableImportItemResult{
				Kind:   "encrypted_variable_file",
				Name:   name,
				Status: "failed",
				Error:  err.Error(),
			})
			continue
		}
		status := "updated"
		if created {
			status = "imported"
			out.Imported++
		} else {
			out.Updated++
		}
		out.Results = append(out.Results, InventoryVariableImportItemResult{
			Kind:   "encrypted_variable_file",
			Name:   name,
			Status: status,
		})
	}
}

func validateNodeStatusFilter(status string) error {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return nil
	}
	switch status {
	case NodeStatusBootstrap, NodeStatusActive, NodeStatusQuarantined, NodeStatusDecommissioned:
		return nil
	default:
		return errors.New("status must be bootstrap, active, quarantined, or decommissioned")
	}
}
