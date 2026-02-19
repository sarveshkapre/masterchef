package control

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/planner"
)

type CanonicalizationInput struct {
	Kind    string          `json:"kind"` // config|plan
	Content json.RawMessage `json:"content"`
}

type CanonicalizationResult struct {
	Kind         string `json:"kind"`
	Canonical    string `json:"canonical"`
	CanonicalSHA string `json:"canonical_sha256"`
	Bytes        int    `json:"bytes"`
}

func CanonicalizeDocument(in CanonicalizationInput) (CanonicalizationResult, error) {
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	if kind != "config" && kind != "plan" {
		return CanonicalizationResult{}, errors.New("kind must be config or plan")
	}
	content := bytes.TrimSpace(in.Content)
	if len(content) == 0 {
		return CanonicalizationResult{}, errors.New("content is required")
	}
	switch kind {
	case "config":
		var cfg config.Config
		if err := json.Unmarshal(content, &cfg); err != nil {
			return CanonicalizationResult{}, fmt.Errorf("invalid config content: %w", err)
		}
	case "plan":
		var p planner.Plan
		if err := json.Unmarshal(content, &p); err != nil {
			return CanonicalizationResult{}, fmt.Errorf("invalid plan content: %w", err)
		}
	}

	dec := json.NewDecoder(bytes.NewReader(content))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return CanonicalizationResult{}, fmt.Errorf("decode content: %w", err)
	}

	canonical, err := canonicalJSON(value)
	if err != nil {
		return CanonicalizationResult{}, err
	}
	sum := sha256.Sum256([]byte(canonical))
	return CanonicalizationResult{
		Kind:         kind,
		Canonical:    canonical,
		CanonicalSHA: hex.EncodeToString(sum[:]),
		Bytes:        len(canonical),
	}, nil
}

func canonicalJSON(v any) (string, error) {
	switch value := v.(type) {
	case nil:
		return "null", nil
	case bool:
		if value {
			return "true", nil
		}
		return "false", nil
	case json.Number:
		return value.String(), nil
	case float64:
		return json.Number(fmt.Sprintf("%g", value)).String(), nil
	case string:
		b, _ := json.Marshal(value)
		return string(b), nil
	case []any:
		var b strings.Builder
		b.WriteByte('[')
		for i, item := range value {
			if i > 0 {
				b.WriteByte(',')
			}
			encoded, err := canonicalJSON(item)
			if err != nil {
				return "", err
			}
			b.WriteString(encoded)
		}
		b.WriteByte(']')
		return b.String(), nil
	case map[string]any:
		keys := make([]string, 0, len(value))
		for k := range value {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			b.Write(kb)
			b.WriteByte(':')
			encoded, err := canonicalJSON(value[k])
			if err != nil {
				return "", err
			}
			b.WriteString(encoded)
		}
		b.WriteByte('}')
		return b.String(), nil
	default:
		blob, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		dec := json.NewDecoder(bytes.NewReader(blob))
		dec.UseNumber()
		var nested any
		if err := dec.Decode(&nested); err != nil {
			return "", err
		}
		return canonicalJSON(nested)
	}
}
