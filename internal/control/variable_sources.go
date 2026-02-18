package control

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type VariableSourceSpec struct {
	Name   string         `json:"name"`
	Type   string         `json:"type"` // inline|env|file|http
	Config map[string]any `json:"config"`
}

type VariableSourceRegistry struct {
	baseDir string
	client  *http.Client
}

func NewVariableSourceRegistry(baseDir string) *VariableSourceRegistry {
	return &VariableSourceRegistry{
		baseDir: baseDir,
		client: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

func (r *VariableSourceRegistry) ResolveLayers(ctx context.Context, specs []VariableSourceSpec) ([]VariableLayer, error) {
	layers := make([]VariableLayer, 0, len(specs))
	for i, spec := range specs {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			name = "source-" + itoa(int64(i+1))
		}
		sourceType := strings.ToLower(strings.TrimSpace(spec.Type))
		if sourceType == "" {
			return nil, errors.New("source type is required")
		}
		var (
			data map[string]any
			err  error
		)
		switch sourceType {
		case "inline":
			data, err = r.resolveInline(spec.Config)
		case "env":
			data, err = r.resolveEnv(spec.Config)
		case "file":
			data, err = r.resolveFile(spec.Config)
		case "http":
			data, err = r.resolveHTTP(ctx, spec.Config)
		default:
			return nil, errors.New("unsupported variable source type: " + sourceType)
		}
		if err != nil {
			return nil, errors.New(name + ": " + err.Error())
		}
		layers = append(layers, VariableLayer{
			Name: name,
			Data: data,
		})
	}
	return layers, nil
}

func (r *VariableSourceRegistry) resolveInline(config map[string]any) (map[string]any, error) {
	dataRaw, ok := config["data"]
	if !ok {
		return nil, errors.New("inline source requires config.data object")
	}
	data, ok := toAnyMap(dataRaw)
	if !ok {
		return nil, errors.New("inline source config.data must be an object")
	}
	return data, nil
}

func (r *VariableSourceRegistry) resolveEnv(config map[string]any) (map[string]any, error) {
	prefix := strings.TrimSpace(stringValue(config["prefix"]))
	keys := stringSlice(config["keys"])
	target := strings.TrimSpace(stringValue(config["target"]))
	out := map[string]any{}

	if len(keys) > 0 {
		for _, key := range keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			v, ok := os.LookupEnv(key)
			if !ok {
				continue
			}
			out[normalizeEnvVarKey(key, prefix)] = v
		}
	} else {
		for _, envKV := range os.Environ() {
			parts := strings.SplitN(envKV, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := parts[0]
			val := parts[1]
			if prefix != "" && !strings.HasPrefix(key, prefix) {
				continue
			}
			out[normalizeEnvVarKey(key, prefix)] = val
		}
	}

	if target == "" {
		return out, nil
	}
	wrapped := map[string]any{}
	setNestedMapValue(wrapped, target, out)
	return wrapped, nil
}

func (r *VariableSourceRegistry) resolveFile(config map[string]any) (map[string]any, error) {
	path := strings.TrimSpace(stringValue(config["path"]))
	if path == "" {
		return nil, errors.New("file source requires config.path")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.baseDir, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseVariablePayload(raw)
}

func (r *VariableSourceRegistry) resolveHTTP(ctx context.Context, config map[string]any) (map[string]any, error) {
	url := strings.TrimSpace(stringValue(config["url"]))
	if url == "" {
		return nil, errors.New("http source requires config.url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if headers, ok := toStringMap(config["headers"]); ok {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("unexpected http status: " + resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseVariablePayload(body)
}

func parseVariablePayload(raw []byte) (map[string]any, error) {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err == nil {
		if out == nil {
			out = map[string]any{}
		}
		return out, nil
	}
	if err := yaml.Unmarshal(raw, &out); err == nil {
		if out == nil {
			out = map[string]any{}
		}
		return out, nil
	}
	return nil, errors.New("payload must be valid json or yaml object")
}

func toAnyMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	if ok {
		return cloneVariableMap(m), true
	}
	buf, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	var out map[string]any
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, false
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, true
}

func toStringMap(v any) (map[string]string, bool) {
	if v == nil {
		return nil, false
	}
	if in, ok := v.(map[string]string); ok {
		out := map[string]string{}
		for k, val := range in {
			out[k] = val
		}
		return out, true
	}
	rawMap, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	out := map[string]string{}
	for k, val := range rawMap {
		out[k] = stringValue(val)
	}
	return out, true
}

func stringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return append([]string{}, t...)
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s := strings.TrimSpace(stringValue(item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func stringValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		buf, _ := json.Marshal(t)
		return strings.Trim(string(buf), "\"")
	}
}

func normalizeEnvVarKey(key, prefix string) string {
	if prefix != "" && strings.HasPrefix(key, prefix) {
		key = strings.TrimPrefix(key, prefix)
	}
	key = strings.Trim(strings.ToLower(key), "_")
	key = strings.ReplaceAll(key, "__", "_")
	return key
}

func setNestedMapValue(dst map[string]any, path string, val any) {
	parts := strings.Split(path, ".")
	cur := dst
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i == len(parts)-1 {
			cur[part] = cloneVariableAny(val)
			return
		}
		next, ok := cur[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[part] = next
		}
		cur = next
	}
}
