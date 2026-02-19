package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleInventoryCMDBImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.CMDBImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	result, err := control.BulkImportFromCMDB(s.nodes, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.recordEvent(control.Event{
		Type:    "inventory.import.cmdb",
		Message: "cmdb import processed",
		Fields: map[string]any{
			"source_system": result.SourceSystem,
			"dry_run":       result.DryRun,
			"imported":      result.Imported,
			"updated":       result.Updated,
			"failed":        result.Failed,
		},
	}, true)
	code := http.StatusOK
	if !req.DryRun {
		code = http.StatusCreated
	}
	writeJSON(w, code, result)
}

func (s *Server) handleInventoryImportAssistant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.ImportAssistantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	item, err := control.BuildImportAssistant(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}
