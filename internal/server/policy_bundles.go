package server

import (
	"encoding/json"
	"net/http"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handlePolicyBundles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.policyBundles.List())
	case http.MethodPost:
		var req control.PolicyBundleInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		bundle, err := s.policyBundles.Create(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "policy.bundle.created",
			Message: "policy bundle created",
			Fields: map[string]any{
				"bundle_id":    bundle.ID,
				"name":         bundle.Name,
				"version":      bundle.Version,
				"policy_group": bundle.PolicyGroup,
				"lock_digest":  bundle.LockDigest,
				"lock_entries": len(bundle.LockEntries),
			},
		}, true)
		writeJSON(w, http.StatusCreated, bundle)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePolicyBundleAction(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	// /v1/policy/bundles/{id}
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "policy" || parts[2] != "bundles" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	bundleID := parts[3]
	switch {
	case len(parts) == 4 && r.Method == http.MethodGet:
		bundle, ok := s.policyBundles.Get(bundleID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy bundle not found"})
			return
		}
		writeJSON(w, http.StatusOK, bundle)
	case len(parts) == 5 && parts[4] == "promote" && r.Method == http.MethodPost:
		var req control.PolicyBundlePromotionInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		promo, err := s.policyBundles.Promote(bundleID, req)
		if err != nil {
			if err.Error() == "policy bundle not found" {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "policy.bundle.promoted",
			Message: "policy bundle promoted",
			Fields: map[string]any{
				"bundle_id":    promo.BundleID,
				"bundle_name":  promo.BundleName,
				"bundle_ver":   promo.BundleVer,
				"from_group":   promo.FromGroup,
				"target_group": promo.TargetGroup,
			},
		}, true)
		writeJSON(w, http.StatusOK, promo)
	case len(parts) == 5 && parts[4] == "promotions" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, s.policyBundles.ListPromotions(bundleID))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
