package cli

import "testing"

func TestRunDocsVerifyExamples(t *testing.T) {
	if err := runDocs([]string{"verify-examples", "-format", "json"}); err != nil {
		t.Fatalf("runDocs verify-examples failed: %v", err)
	}
}
