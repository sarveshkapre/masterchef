package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleDiscoverySources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.discoveryInventory.ListSources())
	case http.MethodPost:
		var req control.DiscoverySourceInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := s.discoveryInventory.CreateSource(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "inventory.discovery.source.created",
			Message: "inventory discovery source registered",
			Fields: map[string]any{
				"source_id": item.ID,
				"kind":      item.Kind,
				"enabled":   item.Enabled,
			},
		}, true)
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDiscoverySourceAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/inventory/discovery-sources/{id}
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "inventory" || parts[2] != "discovery-sources" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	item, ok := s.discoveryInventory.GetSource(parts[3])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "discovery source not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleDiscoverySourceSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.DiscoverySyncInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	source, enrolls, report, err := s.discoveryInventory.PrepareSync(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	created := 0
	updated := 0
	for _, in := range enrolls {
		_, wasCreated, err := s.nodes.Enroll(in)
		if err != nil {
			continue
		}
		if wasCreated {
			created++
		} else {
			updated++
		}
	}
	response := map[string]any{
		"source_id":       report.SourceID,
		"kind":            report.Kind,
		"requested_hosts": report.RequestedHosts,
		"valid_hosts":     report.ValidHosts,
		"created":         created,
		"updated":         updated,
	}
	s.recordEvent(control.Event{
		Type:    "inventory.discovery.sync",
		Message: "inventory discovery sync applied",
		Fields: map[string]any{
			"source_id": source.ID,
			"kind":      source.Kind,
			"created":   created,
			"updated":   updated,
		},
	}, true)
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleCloudInventorySync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Provider      string                   `json:"provider"`
		Region        string                   `json:"region,omitempty"`
		Hosts         []control.DiscoveredHost `json:"hosts"`
		DefaultLabels map[string]string        `json:"default_labels,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	switch provider {
	case control.InventoryDiscoveryAWS, control.InventoryDiscoveryAzure, control.InventoryDiscoveryGCP, control.InventoryDiscoveryVSphere:
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider must be one of aws, azure, gcp, vsphere"})
		return
	}
	region := strings.TrimSpace(req.Region)
	defaultLabels := map[string]string{}
	for k, v := range req.DefaultLabels {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		defaultLabels[key] = strings.TrimSpace(v)
	}
	defaultLabels["cloud_provider"] = provider
	if region != "" {
		defaultLabels["cloud_region"] = region
	}

	requested := len(req.Hosts)
	valid := 0
	created := 0
	updated := 0
	for _, host := range req.Hosts {
		name := strings.TrimSpace(host.Name)
		if name == "" {
			continue
		}
		labels := map[string]string{}
		for k, v := range defaultLabels {
			labels[k] = v
		}
		for k, v := range host.Labels {
			key := strings.ToLower(strings.TrimSpace(k))
			if key == "" {
				continue
			}
			labels[key] = strings.TrimSpace(v)
		}
		transport := strings.ToLower(strings.TrimSpace(host.Transport))
		if transport == "" {
			transport = "ssh"
		}
		_, wasCreated, err := s.nodes.Enroll(control.NodeEnrollInput{
			Name:      name,
			Address:   strings.TrimSpace(host.Address),
			Transport: transport,
			Labels:    labels,
			Roles:     host.Roles,
			Topology:  host.Topology,
			Source:    "cloud:" + provider,
		})
		if err != nil {
			continue
		}
		valid++
		if wasCreated {
			created++
		} else {
			updated++
		}
	}

	resp := map[string]any{
		"provider":        provider,
		"region":          region,
		"requested_hosts": requested,
		"valid_hosts":     valid,
		"created":         created,
		"updated":         updated,
	}
	s.recordEvent(control.Event{
		Type:    "inventory.cloud.sync",
		Message: "cloud inventory sync applied",
		Fields: map[string]any{
			"provider": provider,
			"region":   region,
			"created":  created,
			"updated":  updated,
		},
	}, true)
	writeJSON(w, http.StatusOK, resp)
}
