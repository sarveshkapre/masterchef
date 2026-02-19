package control

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type NodeClassificationRuleInput struct {
	Name        string            `json:"name"`
	MatchFacts  map[string]string `json:"match_facts,omitempty"`
	MatchLabels map[string]string `json:"match_labels,omitempty"`
	PolicyGroup string            `json:"policy_group,omitempty"`
	RunList     []string          `json:"run_list,omitempty"`
	Variables   map[string]any    `json:"variables,omitempty"`
	Priority    int               `json:"priority,omitempty"`
	Enabled     bool              `json:"enabled"`
}

type NodeClassificationRule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	MatchFacts  map[string]string `json:"match_facts,omitempty"`
	MatchLabels map[string]string `json:"match_labels,omitempty"`
	PolicyGroup string            `json:"policy_group,omitempty"`
	RunList     []string          `json:"run_list,omitempty"`
	Variables   map[string]any    `json:"variables,omitempty"`
	Priority    int               `json:"priority"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type NodeClassificationRequest struct {
	Node   string         `json:"node"`
	Facts  map[string]any `json:"facts,omitempty"`
	Labels map[string]any `json:"labels,omitempty"`
}

type NodeClassificationResult struct {
	Node           string         `json:"node"`
	PolicyGroup    string         `json:"policy_group,omitempty"`
	RunList        []string       `json:"run_list,omitempty"`
	Variables      map[string]any `json:"variables,omitempty"`
	MatchedRuleIDs []string       `json:"matched_rule_ids,omitempty"`
}

type NodeClassificationStore struct {
	mu     sync.RWMutex
	nextID int64
	items  map[string]*NodeClassificationRule
}

func NewNodeClassificationStore() *NodeClassificationStore {
	return &NodeClassificationStore{items: map[string]*NodeClassificationRule{}}
}

func (s *NodeClassificationStore) Upsert(in NodeClassificationRuleInput) (NodeClassificationRule, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return NodeClassificationRule{}, errors.New("name is required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if strings.EqualFold(item.Name, name) {
			item.MatchFacts = cloneStringMap(in.MatchFacts)
			item.MatchLabels = cloneStringMap(in.MatchLabels)
			item.PolicyGroup = strings.TrimSpace(in.PolicyGroup)
			item.RunList = normalizeStringSlice(in.RunList)
			item.Variables = cloneAnyMap(in.Variables)
			item.Priority = in.Priority
			item.Enabled = in.Enabled
			item.UpdatedAt = now
			return cloneNodeClassificationRule(*item), nil
		}
	}
	s.nextID++
	item := &NodeClassificationRule{
		ID:          "class-rule-" + itoa(s.nextID),
		Name:        name,
		MatchFacts:  cloneStringMap(in.MatchFacts),
		MatchLabels: cloneStringMap(in.MatchLabels),
		PolicyGroup: strings.TrimSpace(in.PolicyGroup),
		RunList:     normalizeStringSlice(in.RunList),
		Variables:   cloneAnyMap(in.Variables),
		Priority:    in.Priority,
		Enabled:     in.Enabled,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.items[item.ID] = item
	return cloneNodeClassificationRule(*item), nil
}

func (s *NodeClassificationStore) List() []NodeClassificationRule {
	s.mu.RLock()
	out := make([]NodeClassificationRule, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, cloneNodeClassificationRule(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (s *NodeClassificationStore) Get(id string) (NodeClassificationRule, bool) {
	s.mu.RLock()
	item, ok := s.items[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return NodeClassificationRule{}, false
	}
	return cloneNodeClassificationRule(*item), true
}

func (s *NodeClassificationStore) Evaluate(req NodeClassificationRequest) NodeClassificationResult {
	rules := s.List()
	out := NodeClassificationResult{
		Node:           strings.TrimSpace(req.Node),
		RunList:        []string{},
		Variables:      map[string]any{},
		MatchedRuleIDs: []string{},
	}
	seenRunList := map[string]struct{}{}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if !matchesClassificationRule(rule, req) {
			continue
		}
		out.MatchedRuleIDs = append(out.MatchedRuleIDs, rule.ID)
		if out.PolicyGroup == "" && strings.TrimSpace(rule.PolicyGroup) != "" {
			out.PolicyGroup = rule.PolicyGroup
		}
		for _, item := range rule.RunList {
			if _, ok := seenRunList[item]; ok {
				continue
			}
			seenRunList[item] = struct{}{}
			out.RunList = append(out.RunList, item)
		}
		for key, value := range rule.Variables {
			if _, exists := out.Variables[key]; exists {
				continue
			}
			out.Variables[key] = value
		}
	}
	return out
}

func matchesClassificationRule(rule NodeClassificationRule, req NodeClassificationRequest) bool {
	for key, expected := range rule.MatchLabels {
		actual := fmt.Sprint(req.Labels[key])
		if !strings.EqualFold(strings.TrimSpace(actual), strings.TrimSpace(expected)) {
			return false
		}
	}
	for key, expected := range rule.MatchFacts {
		actual := fmt.Sprint(req.Facts[key])
		if !strings.EqualFold(strings.TrimSpace(actual), strings.TrimSpace(expected)) {
			return false
		}
	}
	return true
}

func cloneNodeClassificationRule(in NodeClassificationRule) NodeClassificationRule {
	out := in
	out.MatchFacts = cloneStringMap(in.MatchFacts)
	out.MatchLabels = cloneStringMap(in.MatchLabels)
	out.RunList = append([]string{}, in.RunList...)
	out.Variables = cloneAnyMap(in.Variables)
	return out
}
