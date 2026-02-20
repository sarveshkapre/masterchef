package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleInventoryExportBundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.InventoryVariableExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	bundle, err := control.BuildInventoryVariableBundle(s.nodes, s.roleEnv, s.encryptedVars, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "inventory.export.bundle",
		Message: "inventory and variable bundle exported",
		Fields: map[string]any{
			"nodes":                    len(bundle.Inventory),
			"roles":                    len(bundle.Roles),
			"environments":             len(bundle.Environments),
			"encrypted_variable_files": len(bundle.EncryptedVariableFiles),
		},
	}, true)
	writeJSON(w, http.StatusOK, bundle)
}

func (s *Server) handleInventoryImportBundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.InventoryVariableImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	result, err := control.ImportInventoryVariableBundle(s.nodes, s.roleEnv, s.encryptedVars, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "inventory.import.bundle",
		Message: "inventory and variable bundle import processed",
		Fields: map[string]any{
			"dry_run":                           result.DryRun,
			"inventory_imported":                result.Inventory.Imported,
			"inventory_updated":                 result.Inventory.Updated,
			"inventory_failed":                  result.Inventory.Failed,
			"roles_imported":                    result.Roles.Imported,
			"roles_updated":                     result.Roles.Updated,
			"roles_failed":                      result.Roles.Failed,
			"environments_imported":             result.Environments.Imported,
			"environments_updated":              result.Environments.Updated,
			"environments_failed":               result.Environments.Failed,
			"encrypted_variable_files_imported": result.EncryptedVariableFiles.Imported,
			"encrypted_variable_files_updated":  result.EncryptedVariableFiles.Updated,
			"encrypted_variable_files_failed":   result.EncryptedVariableFiles.Failed,
		},
	}, true)
	code := http.StatusCreated
	if req.DryRun {
		code = http.StatusOK
	}
	writeJSON(w, code, result)
}
