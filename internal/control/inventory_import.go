package control

import (
	"errors"
	"strings"
)

type CMDBRecord struct {
	Name      string            `json:"name"`
	Address   string            `json:"address,omitempty"`
	Transport string            `json:"transport,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Roles     []string          `json:"roles,omitempty"`
	Topology  map[string]string `json:"topology,omitempty"`
}

type CMDBImportRequest struct {
	SourceSystem string       `json:"source_system,omitempty"`
	DryRun       bool         `json:"dry_run,omitempty"`
	Records      []CMDBRecord `json:"records"`
}

type CMDBImportItemResult struct {
	Name   string `json:"name,omitempty"`
	Status string `json:"status"` // imported|updated|would_import|would_update|failed
	Error  string `json:"error,omitempty"`
}

type CMDBImportResult struct {
	SourceSystem string                 `json:"source_system"`
	DryRun       bool                   `json:"dry_run"`
	Imported     int                    `json:"imported"`
	Updated      int                    `json:"updated"`
	Failed       int                    `json:"failed"`
	Results      []CMDBImportItemResult `json:"results"`
}

func BulkImportFromCMDB(nodes *NodeLifecycleStore, req CMDBImportRequest) (CMDBImportResult, error) {
	if nodes == nil {
		return CMDBImportResult{}, errors.New("node lifecycle store is required")
	}
	source := strings.ToLower(strings.TrimSpace(req.SourceSystem))
	if source == "" {
		source = "cmdb"
	}
	if len(req.Records) == 0 {
		return CMDBImportResult{}, errors.New("records are required")
	}

	out := CMDBImportResult{
		SourceSystem: source,
		DryRun:       req.DryRun,
		Results:      make([]CMDBImportItemResult, 0, len(req.Records)),
	}
	sourceLabel := "cmdb:" + source
	for _, record := range req.Records {
		name := strings.TrimSpace(record.Name)
		if name == "" {
			out.Failed++
			out.Results = append(out.Results, CMDBImportItemResult{
				Status: "failed",
				Error:  "record name is required",
			})
			continue
		}
		input := NodeEnrollInput{
			Name:      name,
			Address:   record.Address,
			Transport: record.Transport,
			Labels:    record.Labels,
			Roles:     record.Roles,
			Topology:  record.Topology,
			Source:    sourceLabel,
		}

		if req.DryRun {
			_, exists := nodes.Get(name)
			status := "would_import"
			if exists {
				status = "would_update"
			}
			out.Results = append(out.Results, CMDBImportItemResult{Name: name, Status: status})
			if exists {
				out.Updated++
			} else {
				out.Imported++
			}
			continue
		}

		_, created, err := nodes.Enroll(input)
		if err != nil {
			out.Failed++
			out.Results = append(out.Results, CMDBImportItemResult{
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
		out.Results = append(out.Results, CMDBImportItemResult{Name: name, Status: status})
	}
	return out, nil
}
