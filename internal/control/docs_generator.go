package control

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

type DocumentationGenerateInput struct {
	Format           string `json:"format,omitempty"` // markdown|json
	IncludePackages  bool   `json:"include_packages"`
	IncludePolicyAPI bool   `json:"include_policy_api"`
}

type DocumentationArtifact struct {
	Format      string         `json:"format"`
	Content     string         `json:"content"`
	Sections    []string       `json:"sections"`
	Counts      map[string]int `json:"counts"`
	GeneratedAt time.Time      `json:"generated_at"`
}

func GenerateDocumentation(in DocumentationGenerateInput, packages []PackageArtifact, endpoints []string) DocumentationArtifact {
	format := strings.ToLower(strings.TrimSpace(in.Format))
	if format != "json" {
		format = "markdown"
	}
	if !in.IncludePackages && !in.IncludePolicyAPI {
		in.IncludePackages = true
		in.IncludePolicyAPI = true
	}

	sections := make([]string, 0, 2)
	counts := map[string]int{}
	pkgLines := []string{}
	if in.IncludePackages {
		sections = append(sections, "packages")
		pkgLines = packageDocLines(packages)
		counts["packages"] = len(pkgLines)
	}
	policyLines := []string{}
	if in.IncludePolicyAPI {
		sections = append(sections, "policy_api")
		policyLines = policyEndpointLines(endpoints)
		counts["policy_api"] = len(policyLines)
	}

	out := DocumentationArtifact{
		Format:      format,
		Sections:    sections,
		Counts:      counts,
		GeneratedAt: time.Now().UTC(),
	}
	if format == "json" {
		doc := map[string]any{
			"packages":   pkgLines,
			"policy_api": policyLines,
			"sections":   sections,
			"counts":     counts,
		}
		b, _ := json.MarshalIndent(doc, "", "  ")
		out.Content = string(b)
		return out
	}

	var sb strings.Builder
	sb.WriteString("# Masterchef Generated Documentation\n\n")
	if in.IncludePackages {
		sb.WriteString("## Modules and Providers\n")
		if len(pkgLines) == 0 {
			sb.WriteString("- none\n\n")
		} else {
			for _, line := range pkgLines {
				sb.WriteString("- ")
				sb.WriteString(line)
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
	}
	if in.IncludePolicyAPI {
		sb.WriteString("## Policy APIs\n")
		if len(policyLines) == 0 {
			sb.WriteString("- none\n")
		} else {
			for _, line := range policyLines {
				sb.WriteString("- ")
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
	}
	out.Content = sb.String()
	return out
}

func packageDocLines(packages []PackageArtifact) []string {
	if len(packages) == 0 {
		return nil
	}
	items := append([]PackageArtifact{}, packages...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].Version < items[j].Version
	})
	out := make([]string, 0, len(items))
	for _, item := range items {
		line := item.Kind + " " + item.Name + "@" + item.Version + " (" + item.Digest + ")"
		if item.Signed {
			line += " signed"
		} else {
			line += " unsigned"
		}
		out = append(out, line)
	}
	return out
}

func policyEndpointLines(endpoints []string) []string {
	if len(endpoints) == 0 {
		return nil
	}
	out := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		lower := strings.ToLower(endpoint)
		if strings.Contains(lower, "/policy/") || strings.Contains(lower, "/release/") {
			out = append(out, endpoint)
		}
	}
	sort.Strings(out)
	return out
}
