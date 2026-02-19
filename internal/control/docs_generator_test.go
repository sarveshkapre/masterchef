package control

import (
	"strings"
	"testing"
)

func TestGenerateDocumentationMarkdown(t *testing.T) {
	artifact := GenerateDocumentation(DocumentationGenerateInput{Format: "markdown"}, []PackageArtifact{
		{
			Kind:    "module",
			Name:    "core/network",
			Version: "1.0.0",
			Digest:  "sha256:abc",
			Signed:  true,
		},
	}, []string{
		"POST /v1/policy/pull/execute",
		"GET /v1/healthz",
	})
	if artifact.Format != "markdown" {
		t.Fatalf("expected markdown format")
	}
	if !strings.Contains(artifact.Content, "core/network@1.0.0") {
		t.Fatalf("expected package line in markdown content")
	}
	if !strings.Contains(artifact.Content, "POST /v1/policy/pull/execute") {
		t.Fatalf("expected policy api line in markdown content")
	}
}

func TestGenerateDocumentationJSON(t *testing.T) {
	artifact := GenerateDocumentation(DocumentationGenerateInput{
		Format:           "json",
		IncludePackages:  true,
		IncludePolicyAPI: false,
	}, nil, nil)
	if artifact.Format != "json" {
		t.Fatalf("expected json format")
	}
	if !strings.Contains(artifact.Content, "\"packages\"") {
		t.Fatalf("expected json packages field")
	}
}
