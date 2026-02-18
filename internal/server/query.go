package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/state"
)

type queryNode struct {
	Op         string      `json:"op,omitempty"`
	Field      string      `json:"field,omitempty"`
	Comparator string      `json:"comparator,omitempty"`
	Value      any         `json:"value,omitempty"`
	Conditions []queryNode `json:"conditions,omitempty"`
}

func (s *Server) handleQuery(baseDir string) http.HandlerFunc {
	type reqBody struct {
		Entity   string     `json:"entity"`
		Mode     string     `json:"mode"` // human|ast
		Query    string     `json:"query"`
		QueryAST *queryNode `json:"query_ast"`
		Limit    int        `json:"limit"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}

		entity := strings.ToLower(strings.TrimSpace(req.Entity))
		if entity == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "entity is required"})
			return
		}
		mode := strings.ToLower(strings.TrimSpace(req.Mode))
		if mode == "" {
			mode = "human"
		}
		if req.Limit <= 0 {
			req.Limit = 100
		}

		records, err := s.queryEntityRecords(entity, baseDir)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		var root *queryNode
		switch mode {
		case "human":
			parsed, err := parseHumanQuery(req.Query)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			root = parsed
		case "ast":
			root = req.QueryAST
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mode must be human or ast"})
			return
		}

		matched := make([]any, 0, minInt(req.Limit, len(records)))
		for _, rec := range records {
			m, err := toMap(rec)
			if err != nil {
				continue
			}
			ok, err := matchNode(m, root)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if ok {
				matched = append(matched, rec)
				if len(matched) >= req.Limit {
					break
				}
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"entity":        entity,
			"mode":          mode,
			"total":         len(records),
			"matched_count": len(matched),
			"items":         matched,
			"ast":           root,
		})
	}
}

func (s *Server) queryEntityRecords(entity, baseDir string) ([]any, error) {
	switch entity {
	case "events":
		events := s.events.List()
		out := make([]any, 0, len(events))
		for _, e := range events {
			out = append(out, e)
		}
		return out, nil
	case "alerts":
		alerts := s.alerts.List("all", 2000)
		out := make([]any, 0, len(alerts))
		for _, a := range alerts {
			out = append(out, a)
		}
		return out, nil
	case "change_records":
		records := s.changeRecords.List()
		out := make([]any, 0, len(records))
		for _, rec := range records {
			out = append(out, rec)
		}
		return out, nil
	case "runbooks":
		runbooks := s.runbooks.List()
		out := make([]any, 0, len(runbooks))
		for _, rb := range runbooks {
			out = append(out, rb)
		}
		return out, nil
	case "checklists":
		items := s.checklists.List()
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, item)
		}
		return out, nil
	case "views":
		views := s.views.List()
		out := make([]any, 0, len(views))
		for _, view := range views {
			out = append(out, view)
		}
		return out, nil
	case "migration_reports":
		reports := s.migrations.List()
		out := make([]any, 0, len(reports))
		for _, report := range reports {
			out = append(out, report)
		}
		return out, nil
	case "solution_packs":
		packs := s.solutionPacks.List()
		out := make([]any, 0, len(packs))
		for _, p := range packs {
			out = append(out, p)
		}
		return out, nil
	case "workspace_templates":
		templates := s.workspaceTemplates.List()
		out := make([]any, 0, len(templates))
		for _, tpl := range templates {
			out = append(out, tpl)
		}
		return out, nil
	case "use_case_templates":
		templates := s.useCaseTemplates.List()
		out := make([]any, 0, len(templates))
		for _, tpl := range templates {
			out = append(out, tpl)
		}
		return out, nil
	case "jobs":
		jobs := s.queue.List()
		out := make([]any, 0, len(jobs))
		for _, j := range jobs {
			out = append(out, j)
		}
		return out, nil
	case "runs":
		runs, err := state.New(baseDir).ListRuns(2000)
		if err != nil {
			return nil, err
		}
		out := make([]any, 0, len(runs))
		for _, run := range runs {
			out = append(out, run)
		}
		return out, nil
	case "workflow_runs":
		runs := s.workflows.ListRuns()
		out := make([]any, 0, len(runs))
		for _, run := range runs {
			out = append(out, run)
		}
		return out, nil
	case "associations":
		list := s.assocs.List()
		out := make([]any, 0, len(list))
		for _, item := range list {
			out = append(out, item)
		}
		return out, nil
	case "templates":
		list := s.templates.List()
		out := make([]any, 0, len(list))
		for _, item := range list {
			out = append(out, item)
		}
		return out, nil
	case "schedules":
		list := s.scheduler.List()
		out := make([]any, 0, len(list))
		for _, item := range list {
			out = append(out, item)
		}
		return out, nil
	default:
		return nil, errors.New("unsupported entity")
	}
}

func parseHumanQuery(q string) (*queryNode, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	orSplit := regexp.MustCompile(`(?i)\s+OR\s+`).Split(q, -1)
	orNodes := make([]queryNode, 0, len(orSplit))
	for _, orPart := range orSplit {
		andSplit := regexp.MustCompile(`(?i)\s+AND\s+`).Split(strings.TrimSpace(orPart), -1)
		andNodes := make([]queryNode, 0, len(andSplit))
		for _, term := range andSplit {
			n, err := parseHumanTerm(strings.TrimSpace(term))
			if err != nil {
				return nil, err
			}
			andNodes = append(andNodes, n)
		}
		if len(andNodes) == 1 {
			orNodes = append(orNodes, andNodes[0])
		} else {
			orNodes = append(orNodes, queryNode{Op: "and", Conditions: andNodes})
		}
	}
	if len(orNodes) == 1 {
		return &orNodes[0], nil
	}
	root := queryNode{Op: "or", Conditions: orNodes}
	return &root, nil
}

func parseHumanTerm(term string) (queryNode, error) {
	if term == "" {
		return queryNode{}, errors.New("empty query term")
	}
	for _, item := range []struct {
		token string
		op    string
	}{
		{"!=", "ne"},
		{">=", "gte"},
		{"<=", "lte"},
		{"~=", "contains"},
		{"~", "contains"},
		{"=", "eq"},
		{">", "gt"},
		{"<", "lt"},
	} {
		if idx := strings.Index(term, item.token); idx > 0 {
			field := strings.TrimSpace(term[:idx])
			value := strings.TrimSpace(term[idx+len(item.token):])
			value = strings.Trim(value, "'\"")
			if field == "" {
				break
			}
			return queryNode{Field: field, Comparator: item.op, Value: value}, nil
		}
	}
	return queryNode{}, fmt.Errorf("invalid query term: %s", term)
}

func matchNode(rec map[string]any, node *queryNode) (bool, error) {
	if node == nil {
		return true, nil
	}
	op := strings.ToLower(strings.TrimSpace(node.Op))
	switch op {
	case "and":
		for i := range node.Conditions {
			ok, err := matchNode(rec, &node.Conditions[i])
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}
		return true, nil
	case "or":
		for i := range node.Conditions {
			ok, err := matchNode(rec, &node.Conditions[i])
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	case "":
		return compareField(rec, node.Field, node.Comparator, node.Value)
	default:
		return false, errors.New("unsupported ast op: " + op)
	}
}

func compareField(rec map[string]any, field, comparator string, expected any) (bool, error) {
	actual, ok := getField(rec, field)
	if !ok {
		return false, nil
	}
	comp := strings.ToLower(strings.TrimSpace(comparator))
	if comp == "" {
		comp = "eq"
	}

	aStr := strings.TrimSpace(fmt.Sprintf("%v", actual))
	eStr := strings.TrimSpace(fmt.Sprintf("%v", expected))

	switch comp {
	case "eq":
		return strings.EqualFold(aStr, eStr), nil
	case "ne":
		return !strings.EqualFold(aStr, eStr), nil
	case "contains":
		return strings.Contains(strings.ToLower(aStr), strings.ToLower(eStr)), nil
	case "prefix":
		return strings.HasPrefix(strings.ToLower(aStr), strings.ToLower(eStr)), nil
	case "suffix":
		return strings.HasSuffix(strings.ToLower(aStr), strings.ToLower(eStr)), nil
	case "gt", "gte", "lt", "lte":
		af, aok := toFloat(actual)
		ef, eok := toFloat(expected)
		if !aok || !eok {
			return false, nil
		}
		switch comp {
		case "gt":
			return af > ef, nil
		case "gte":
			return af >= ef, nil
		case "lt":
			return af < ef, nil
		default:
			return af <= ef, nil
		}
	default:
		return false, errors.New("unsupported comparator: " + comp)
	}
}

func getField(rec map[string]any, fieldPath string) (any, bool) {
	fieldPath = strings.TrimSpace(fieldPath)
	if fieldPath == "" {
		return nil, false
	}
	parts := strings.Split(fieldPath, ".")
	var cur any = rec
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := m[part]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

func toMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
