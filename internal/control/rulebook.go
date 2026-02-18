package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type RuleCondition struct {
	Field      string `json:"field"`
	Comparator string `json:"comparator"`
	Value      string `json:"value"`
}

type RuleAction struct {
	Type       string `json:"type"` // enqueue_apply|launch_template|launch_workflow
	ConfigPath string `json:"config_path,omitempty"`
	TemplateID string `json:"template_id,omitempty"`
	WorkflowID string `json:"workflow_id,omitempty"`
	Priority   string `json:"priority,omitempty"`
	Force      bool   `json:"force,omitempty"`
}

type Rule struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	SourcePrefix    string          `json:"source_prefix"`
	Enabled         bool            `json:"enabled"`
	MatchMode       string          `json:"match_mode"` // all|any
	Conditions      []RuleCondition `json:"conditions,omitempty"`
	Actions         []RuleAction    `json:"actions"`
	CooldownSeconds int             `json:"cooldown_seconds,omitempty"`
	LastTriggeredAt time.Time       `json:"last_triggered_at,omitempty"`
	TriggerCount    int64           `json:"trigger_count"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type RuleMatch struct {
	RuleID   string       `json:"rule_id"`
	RuleName string       `json:"rule_name"`
	Event    Event        `json:"event"`
	Actions  []RuleAction `json:"actions"`
}

type RuleEngine struct {
	mu     sync.RWMutex
	nextID int64
	rules  map[string]*Rule
}

func NewRuleEngine() *RuleEngine {
	return &RuleEngine{rules: map[string]*Rule{}}
}

func (r *RuleEngine) Create(in Rule) (Rule, error) {
	if strings.TrimSpace(in.Name) == "" {
		return Rule{}, errors.New("rule name is required")
	}
	if strings.TrimSpace(in.SourcePrefix) == "" {
		return Rule{}, errors.New("source_prefix is required")
	}
	if len(in.Actions) == 0 {
		return Rule{}, errors.New("at least one action is required")
	}
	in.MatchMode = normalizeMatchMode(in.MatchMode)
	for i := range in.Actions {
		if err := validateRuleAction(&in.Actions[i]); err != nil {
			return Rule{}, err
		}
	}
	for i := range in.Conditions {
		if err := validateRuleCondition(&in.Conditions[i]); err != nil {
			return Rule{}, err
		}
	}
	if in.CooldownSeconds < 0 {
		in.CooldownSeconds = 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	now := time.Now().UTC()
	in.ID = "rule-" + itoa(r.nextID)
	in.SourcePrefix = strings.TrimSpace(in.SourcePrefix)
	if !in.Enabled {
		in.Enabled = true
	}
	in.CreatedAt = now
	in.UpdatedAt = now
	cp := cloneRule(in)
	r.rules[in.ID] = &cp
	return cp, nil
}

func (r *RuleEngine) List() []Rule {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Rule, 0, len(r.rules))
	for _, rule := range r.rules {
		out = append(out, cloneRule(*rule))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *RuleEngine) Get(id string) (Rule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rule, ok := r.rules[id]
	if !ok {
		return Rule{}, errors.New("rule not found")
	}
	return cloneRule(*rule), nil
}

func (r *RuleEngine) SetEnabled(id string, enabled bool) (Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rule, ok := r.rules[id]
	if !ok {
		return Rule{}, errors.New("rule not found")
	}
	rule.Enabled = enabled
	rule.UpdatedAt = time.Now().UTC()
	return cloneRule(*rule), nil
}

func (r *RuleEngine) Evaluate(event Event) ([]RuleMatch, error) {
	ruleIDs := make([]string, 0)
	r.mu.RLock()
	for id := range r.rules {
		ruleIDs = append(ruleIDs, id)
	}
	r.mu.RUnlock()
	sort.Strings(ruleIDs)

	eventMap, err := eventToMap(event)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	matches := make([]RuleMatch, 0)
	for _, id := range ruleIDs {
		r.mu.Lock()
		rule, ok := r.rules[id]
		if !ok {
			r.mu.Unlock()
			continue
		}
		if !rule.Enabled || !strings.HasPrefix(event.Type, rule.SourcePrefix) {
			r.mu.Unlock()
			continue
		}
		if rule.CooldownSeconds > 0 && !rule.LastTriggeredAt.IsZero() {
			next := rule.LastTriggeredAt.Add(time.Duration(rule.CooldownSeconds) * time.Second)
			if now.Before(next) {
				r.mu.Unlock()
				continue
			}
		}
		matched, err := ruleMatchesEvent(rule, eventMap)
		if err != nil {
			r.mu.Unlock()
			return nil, err
		}
		if !matched {
			r.mu.Unlock()
			continue
		}
		rule.LastTriggeredAt = now
		rule.TriggerCount++
		rule.UpdatedAt = now
		match := RuleMatch{
			RuleID:   rule.ID,
			RuleName: rule.Name,
			Event:    event,
			Actions:  append([]RuleAction{}, rule.Actions...),
		}
		r.mu.Unlock()
		matches = append(matches, match)
	}
	return matches, nil
}

func ruleMatchesEvent(rule *Rule, event map[string]any) (bool, error) {
	if rule == nil {
		return false, nil
	}
	if len(rule.Conditions) == 0 {
		return true, nil
	}
	mode := normalizeMatchMode(rule.MatchMode)
	if mode == "any" {
		for _, cond := range rule.Conditions {
			ok, err := evaluateCondition(event, cond)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	for _, cond := range rule.Conditions {
		ok, err := evaluateCondition(event, cond)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func evaluateCondition(event map[string]any, cond RuleCondition) (bool, error) {
	actual, ok := getPathValue(event, cond.Field)
	if !ok {
		return false, nil
	}
	actualStr := strings.TrimSpace(fmt.Sprintf("%v", actual))
	expected := strings.TrimSpace(cond.Value)
	comp := strings.ToLower(strings.TrimSpace(cond.Comparator))
	if comp == "" {
		comp = "eq"
	}
	switch comp {
	case "eq":
		return strings.EqualFold(actualStr, expected), nil
	case "ne":
		return !strings.EqualFold(actualStr, expected), nil
	case "contains":
		return strings.Contains(strings.ToLower(actualStr), strings.ToLower(expected)), nil
	case "prefix":
		return strings.HasPrefix(strings.ToLower(actualStr), strings.ToLower(expected)), nil
	case "suffix":
		return strings.HasSuffix(strings.ToLower(actualStr), strings.ToLower(expected)), nil
	case "regex":
		re, err := regexp.Compile(expected)
		if err != nil {
			return false, err
		}
		return re.MatchString(actualStr), nil
	default:
		return false, errors.New("unsupported comparator: " + comp)
	}
}

func validateRuleAction(action *RuleAction) error {
	if action == nil {
		return errors.New("rule action is required")
	}
	action.Type = strings.ToLower(strings.TrimSpace(action.Type))
	action.Priority = normalizePriority(action.Priority)
	switch action.Type {
	case "enqueue_apply":
		if strings.TrimSpace(action.ConfigPath) == "" {
			return errors.New("enqueue_apply action requires config_path")
		}
	case "launch_template":
		if strings.TrimSpace(action.TemplateID) == "" {
			return errors.New("launch_template action requires template_id")
		}
	case "launch_workflow":
		if strings.TrimSpace(action.WorkflowID) == "" {
			return errors.New("launch_workflow action requires workflow_id")
		}
	default:
		return errors.New("unsupported rule action type: " + action.Type)
	}
	return nil
}

func validateRuleCondition(cond *RuleCondition) error {
	if cond == nil {
		return errors.New("rule condition is required")
	}
	cond.Field = strings.TrimSpace(cond.Field)
	if cond.Field == "" {
		return errors.New("rule condition field is required")
	}
	cond.Comparator = strings.ToLower(strings.TrimSpace(cond.Comparator))
	if cond.Comparator == "" {
		cond.Comparator = "eq"
	}
	switch cond.Comparator {
	case "eq", "ne", "contains", "prefix", "suffix", "regex":
	default:
		return errors.New("unsupported rule condition comparator: " + cond.Comparator)
	}
	return nil
}

func normalizeMatchMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "any") {
		return "any"
	}
	return "all"
}

func eventToMap(event Event) (map[string]any, error) {
	b, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func getPathValue(root map[string]any, path string) (any, bool) {
	if root == nil {
		return nil, false
	}
	parts := strings.Split(strings.TrimSpace(path), ".")
	var cur any = root
	for _, p := range parts {
		if p == "" {
			return nil, false
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := m[p]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func cloneRule(in Rule) Rule {
	out := in
	out.Conditions = append([]RuleCondition{}, in.Conditions...)
	out.Actions = append([]RuleAction{}, in.Actions...)
	return out
}
