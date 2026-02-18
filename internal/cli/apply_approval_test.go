package cli

import (
	"os"
	"testing"

	"github.com/masterchef/masterchef/internal/planner"
)

func TestRequireApplyApproval_AutoApprove(t *testing.T) {
	err := requireApplyApproval(&planner.Plan{}, true, false)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRequireApplyApproval_NonInteractiveFails(t *testing.T) {
	err := requireApplyApproval(&planner.Plan{}, false, true)
	if err == nil {
		t.Fatalf("expected approval failure")
	}
	if ec, ok := err.(ExitError); !ok || ec.Code != 5 {
		t.Fatalf("expected ExitError code 5, got %#v", err)
	}
}

func TestRequireApplyApproval_CIFailsWithoutYes(t *testing.T) {
	prev := os.Getenv("CI")
	_ = os.Setenv("CI", "true")
	defer func() { _ = os.Setenv("CI", prev) }()

	err := requireApplyApproval(&planner.Plan{}, false, false)
	if err == nil {
		t.Fatalf("expected CI approval failure")
	}
	if ec, ok := err.(ExitError); !ok || ec.Code != 5 {
		t.Fatalf("expected ExitError code 5, got %#v", err)
	}
}
