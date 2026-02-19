package server

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleActionDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	items := s.actionDocs.List()
	if q := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q"))); q != "" {
		filtered := make([]any, 0, len(items))
		for _, item := range items {
			text := strings.ToLower(item.ID + " " + item.Title + " " + item.Summary + " " + strings.Join(item.Tags, " "))
			if strings.Contains(text, q) {
				filtered = append(filtered, item)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": filtered,
			"count": len(filtered),
			"query": q,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}

func (s *Server) handleActionDocByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid action docs path"})
		return
	}
	id := parts[3]
	item, err := s.actionDocs.Get(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

type inlineDocMatch struct {
	Doc             control.ActionDoc `json:"doc"`
	Score           int               `json:"score"`
	MatchedEndpoint string            `json:"matched_endpoint,omitempty"`
	Reason          string            `json:"reason"`
}

func (s *Server) handleInlineDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	method, path := parseInlineEndpointQuery(r.URL.Query().Get("endpoint"), r.URL.Query().Get("method"), r.URL.Query().Get("path"))
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

	limit := 5
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 20 {
		limit = 20
	}

	items := s.actionDocs.List()
	matches := make([]inlineDocMatch, 0, len(items))
	for _, doc := range items {
		bestScore := 0
		bestEndpoint := ""
		reason := ""

		if path != "" {
			for _, endpoint := range doc.Endpoints {
				score, matched := inlineEndpointScore(method, path, endpoint)
				if score > bestScore {
					bestScore = score
					bestEndpoint = matched
					reason = "endpoint"
				}
			}
		}
		if query != "" {
			text := strings.ToLower(doc.ID + " " + doc.Title + " " + doc.Summary + " " + strings.Join(doc.Tags, " "))
			if strings.Contains(text, query) {
				score := 40
				if strings.Contains(strings.ToLower(doc.ID), query) {
					score = 60
				}
				if score > bestScore {
					bestScore = score
					reason = "query"
				}
			}
		}
		if bestScore <= 0 {
			continue
		}
		matches = append(matches, inlineDocMatch{
			Doc:             doc,
			Score:           bestScore,
			MatchedEndpoint: bestEndpoint,
			Reason:          reason,
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Doc.ID < matches[j].Doc.ID
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": matches,
		"count": len(matches),
		"query": map[string]string{
			"endpoint": strings.TrimSpace(r.URL.Query().Get("endpoint")),
			"method":   method,
			"path":     path,
			"q":        query,
		},
		"inline_examples": true,
	})
}

func parseInlineEndpointQuery(endpointRaw, methodRaw, pathRaw string) (string, string) {
	method := strings.ToUpper(strings.TrimSpace(methodRaw))
	path := normalizeInlinePath(pathRaw)
	endpoint := strings.TrimSpace(endpointRaw)
	if endpoint != "" {
		parts := strings.Fields(endpoint)
		if len(parts) >= 2 {
			if method == "" {
				method = strings.ToUpper(strings.TrimSpace(parts[0]))
			}
			if path == "" {
				path = normalizeInlinePath(parts[1])
			}
		} else if path == "" {
			path = normalizeInlinePath(endpoint)
		}
	}
	return method, path
}

func inlineEndpointScore(method, path, candidate string) (int, string) {
	cMethod, cPath := parseInlineEndpointQuery(candidate, "", "")
	if cPath == "" || path == "" {
		return 0, ""
	}
	if !inlinePathTemplateMatches(cPath, path) {
		return 0, ""
	}
	score := 75
	if method != "" && cMethod != "" {
		if method == cMethod {
			score = 100
		} else {
			score = 10
		}
	}
	return score, strings.TrimSpace(candidate)
}

func inlinePathTemplateMatches(templatePath, actualPath string) bool {
	tParts := inlinePathSegments(templatePath)
	aParts := inlinePathSegments(actualPath)
	if len(tParts) != len(aParts) {
		return false
	}
	for i := 0; i < len(tParts); i++ {
		t := tParts[i]
		if strings.HasPrefix(t, "{") && strings.HasSuffix(t, "}") {
			continue
		}
		if t != aParts[i] {
			return false
		}
	}
	return true
}

func inlinePathSegments(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	parts := strings.Split(path, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func normalizeInlinePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.ReplaceAll(path, "//", "/")
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	if len(path) > 1 {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}
