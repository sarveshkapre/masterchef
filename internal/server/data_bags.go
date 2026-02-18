package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleDataBags(w http.ResponseWriter, r *http.Request) {
	type upsertReq struct {
		Bag        string         `json:"bag"`
		Item       string         `json:"item"`
		Data       map[string]any `json:"data"`
		Encrypted  bool           `json:"encrypted"`
		Passphrase string         `json:"passphrase"`
		Tags       []string       `json:"tags"`
	}
	switch r.Method {
	case http.MethodGet:
		bag := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("bag")))
		all := s.dataBags.ListSummaries()
		items := make([]control.DataBagItemSummary, 0, len(all))
		for _, item := range all {
			if bag != "" && strings.ToLower(strings.TrimSpace(item.Bag)) != bag {
				continue
			}
			items = append(items, item)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"bags":  s.dataBags.ListBags(),
			"count": len(items),
			"items": items,
		})
	case http.MethodPost:
		var req upsertReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		item, err := s.dataBags.Upsert(req.Bag, req.Item, req.Data, req.Encrypted, req.Passphrase, req.Tags)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDataBagItem(w http.ResponseWriter, r *http.Request) {
	type upsertReq struct {
		Data       map[string]any `json:"data"`
		Encrypted  bool           `json:"encrypted"`
		Passphrase string         `json:"passphrase"`
		Tags       []string       `json:"tags"`
	}

	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid data bag item path"})
		return
	}
	bag := parts[2]
	item := parts[3]

	switch r.Method {
	case http.MethodGet:
		passphrase := strings.TrimSpace(r.URL.Query().Get("passphrase"))
		out, err := s.dataBags.Get(bag, item, passphrase)
		if err != nil {
			writeDataBagError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPut:
		var req upsertReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		out, err := s.dataBags.Upsert(bag, item, req.Data, req.Encrypted, req.Passphrase, req.Tags)
		if err != nil {
			writeDataBagError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodDelete:
		if !s.dataBags.Delete(bag, item) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "data bag item not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDataBagSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.DataBagSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	items, err := s.dataBags.Search(req)
	if err != nil {
		writeDataBagError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count": len(items),
		"items": items,
		"query": req,
	})
}

func writeDataBagError(w http.ResponseWriter, err error) {
	if strings.Contains(strings.ToLower(err.Error()), "not found") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}
