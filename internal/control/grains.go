package control

import "strings"

type GrainRecord struct {
	Node      string         `json:"node"`
	Grains    map[string]any `json:"grains"`
	UpdatedAt string         `json:"updated_at,omitempty"`
}

type GrainQueryInput struct {
	Grain    string `json:"grain,omitempty"`
	Equals   string `json:"equals,omitempty"`
	Contains string `json:"contains,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

func FactRecordToGrains(item FactRecord) GrainRecord {
	grains := cloneFactMap(item.Facts)
	grains["id"] = item.Node
	if _, ok := grains["host"]; !ok {
		grains["host"] = item.Node
	}
	if _, ok := grains["nodename"]; !ok {
		grains["nodename"] = item.Node
	}
	return GrainRecord{
		Node:      item.Node,
		Grains:    grains,
		UpdatedAt: item.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func GrainQueryToFactQuery(in GrainQueryInput) FactCacheQuery {
	return FactCacheQuery{
		Field:    normalizeGrainField(in.Grain),
		Equals:   strings.TrimSpace(in.Equals),
		Contains: strings.TrimSpace(in.Contains),
		Limit:    in.Limit,
	}
}

func normalizeGrainField(field string) string {
	field = strings.TrimSpace(field)
	switch strings.ToLower(field) {
	case "id", "host", "nodename":
		return ""
	default:
		return field
	}
}
