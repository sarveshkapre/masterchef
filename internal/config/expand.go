package config

import (
	"regexp"
	"sort"
	"strings"
)

var matrixTokenPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

func expandConfigResources(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	cfg.Resources = expandResourceCollection(cfg.Resources)
	cfg.Handlers = expandResourceCollection(cfg.Handlers)
	return cfg
}

func expandResourceCollection(resources []Resource) []Resource {
	if len(resources) == 0 {
		return resources
	}
	out := make([]Resource, 0, len(resources))
	for _, in := range resources {
		effectiveMatrix, expanded := expansionMatrix(in)
		combos := matrixCombinations(effectiveMatrix)
		if len(combos) == 0 {
			combos = []map[string]string{{}}
		}
		for _, vars := range combos {
			if !evaluateResourceWhen(in.When, vars) {
				continue
			}
			res := cloneResource(in)
			res.When = ""
			res.Matrix = nil
			res.Loop = nil
			res.LoopVar = ""
			applyResourceTemplateVars(&res, vars)
			if expanded && strings.TrimSpace(res.ID) == strings.TrimSpace(in.ID) {
				res.ID = appendMatrixSuffix(res.ID, vars)
			}
			out = append(out, res)
		}
	}
	return out
}

func expansionMatrix(in Resource) (map[string][]string, bool) {
	matrix := map[string][]string{}
	expanded := false
	for key, values := range in.Matrix {
		matrix[key] = append([]string{}, values...)
		if len(values) > 0 {
			expanded = true
		}
	}
	if len(in.Loop) > 0 {
		key := strings.TrimSpace(in.LoopVar)
		if key == "" {
			key = "item"
		}
		if _, exists := matrix[key]; !exists {
			matrix[key] = append([]string{}, in.Loop...)
			expanded = true
		}
	}
	if len(matrix) == 0 {
		return nil, false
	}
	return matrix, expanded
}

func matrixCombinations(matrix map[string][]string) []map[string]string {
	if len(matrix) == 0 {
		return nil
	}
	keys := make([]string, 0, len(matrix))
	normalized := map[string][]string{}
	for key, values := range matrix {
		name := strings.TrimSpace(key)
		if name == "" {
			continue
		}
		clean := make([]string, 0, len(values))
		seen := map[string]struct{}{}
		for _, v := range values {
			item := strings.TrimSpace(v)
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			clean = append(clean, item)
		}
		if len(clean) == 0 {
			return nil
		}
		sort.Strings(clean)
		normalized[name] = clean
		keys = append(keys, name)
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)

	combos := make([]map[string]string, 0)
	var walk func(int, map[string]string)
	walk = func(idx int, current map[string]string) {
		if idx >= len(keys) {
			item := map[string]string{}
			for k, v := range current {
				item[k] = v
			}
			combos = append(combos, item)
			return
		}
		key := keys[idx]
		for _, value := range normalized[key] {
			current[key] = value
			walk(idx+1, current)
		}
		delete(current, key)
	}
	walk(0, map[string]string{})
	return combos
}

func evaluateResourceWhen(when string, vars map[string]string) bool {
	expr := strings.TrimSpace(when)
	if expr == "" {
		return true
	}
	lower := strings.ToLower(expr)
	switch lower {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	if strings.Contains(expr, "==") {
		parts := strings.SplitN(expr, "==", 2)
		left := resolveWhenOperand(parts[0], vars)
		right := resolveWhenOperand(parts[1], vars)
		return left == right
	}
	if strings.Contains(expr, "!=") {
		parts := strings.SplitN(expr, "!=", 2)
		left := resolveWhenOperand(parts[0], vars)
		right := resolveWhenOperand(parts[1], vars)
		return left != right
	}
	value := resolveWhenOperand(expr, vars)
	value = strings.ToLower(strings.TrimSpace(value))
	return value != "" && value != "0" && value != "false" && value != "no" && value != "off"
}

func resolveWhenOperand(token string, vars map[string]string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if len(token) >= 2 {
		if (token[0] == '\'' && token[len(token)-1] == '\'') || (token[0] == '"' && token[len(token)-1] == '"') {
			return token[1 : len(token)-1]
		}
	}
	if v, ok := vars[token]; ok {
		return v
	}
	return token
}

func applyResourceTemplateVars(res *Resource, vars map[string]string) {
	if res == nil || len(vars) == 0 {
		return
	}
	replaceString := func(v string) string {
		return matrixTokenPattern.ReplaceAllStringFunc(v, func(token string) string {
			match := matrixTokenPattern.FindStringSubmatch(token)
			if len(match) != 2 {
				return token
			}
			key := strings.TrimSpace(match[1])
			if value, ok := vars[key]; ok {
				return value
			}
			return ""
		})
	}
	replaceSlice := func(in []string) []string {
		if len(in) == 0 {
			return in
		}
		out := make([]string, 0, len(in))
		for _, item := range in {
			out = append(out, replaceString(item))
		}
		return out
	}

	res.ID = replaceString(res.ID)
	res.Type = replaceString(res.Type)
	res.Host = replaceString(res.Host)
	res.DelegateTo = replaceString(res.DelegateTo)
	res.Path = replaceString(res.Path)
	res.Content = replaceString(res.Content)
	res.Mode = replaceString(res.Mode)
	res.ContentChecksum = replaceString(res.ContentChecksum)
	res.ContentSignature = replaceString(res.ContentSignature)
	res.ContentSigningPubKey = replaceString(res.ContentSigningPubKey)
	res.Command = replaceString(res.Command)
	res.Creates = replaceString(res.Creates)
	res.OnlyIf = replaceString(res.OnlyIf)
	res.Unless = replaceString(res.Unless)
	res.RefreshCommand = replaceString(res.RefreshCommand)
	res.BecomeUser = replaceString(res.BecomeUser)
	res.RescueCommand = replaceString(res.RescueCommand)
	res.AlwaysCommand = replaceString(res.AlwaysCommand)
	res.RetryBackoff = replaceString(res.RetryBackoff)
	res.UntilContains = replaceString(res.UntilContains)
	res.RegistryKey = replaceString(res.RegistryKey)
	res.RegistryValue = replaceString(res.RegistryValue)
	res.RegistryValueType = replaceString(res.RegistryValueType)
	res.TaskName = replaceString(res.TaskName)
	res.TaskSchedule = replaceString(res.TaskSchedule)
	res.TaskCommand = replaceString(res.TaskCommand)
	res.DependsOn = replaceSlice(res.DependsOn)
	res.Require = replaceSlice(res.Require)
	res.Before = replaceSlice(res.Before)
	res.Notify = replaceSlice(res.Notify)
	res.Subscribe = replaceSlice(res.Subscribe)
	res.NotifyHandlers = replaceSlice(res.NotifyHandlers)
	res.Tags = replaceSlice(res.Tags)
}

func appendMatrixSuffix(id string, vars map[string]string) string {
	base := sanitizeMatrixToken(id)
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)*2+1)
	parts = append(parts, base)
	for _, key := range keys {
		parts = append(parts, sanitizeMatrixToken(key), sanitizeMatrixToken(vars[key]))
	}
	return strings.Join(parts, "-")
}

func sanitizeMatrixToken(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "item"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "item"
	}
	return out
}
