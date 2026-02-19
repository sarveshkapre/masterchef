package control

import "testing"

func TestPackageManagerResolve(t *testing.T) {
	store := NewPackageManagerAbstractionStore()
	resolved := store.Resolve(PackageManagerResolveInput{
		OS:             "linux",
		Distro:         "ubuntu",
		RequiredAction: "install",
	})
	if !resolved.Compatible || resolved.Selected != "apt" {
		t.Fatalf("expected apt resolver on ubuntu, got %+v", resolved)
	}

	resolved = store.Resolve(PackageManagerResolveInput{
		OS:             "windows",
		Preferred:      "chocolatey",
		RequiredAction: "upgrade",
	})
	if !resolved.Compatible || resolved.Selected != "chocolatey" {
		t.Fatalf("expected chocolatey selection, got %+v", resolved)
	}
}

func TestPackageManagerRenderAction(t *testing.T) {
	store := NewPackageManagerAbstractionStore()
	plan, err := store.RenderAction(PackageManagerActionInput{
		OS:      "linux",
		Distro:  "ubuntu",
		Action:  "install",
		Package: "nginx",
		Version: "1.24.0",
	})
	if err != nil {
		t.Fatalf("render install action failed: %v", err)
	}
	if !plan.Allowed || plan.Manager != "apt" {
		t.Fatalf("expected apt install action plan, got %+v", plan)
	}
	if len(plan.Command) == 0 || plan.Command[0] != "apt-get" {
		t.Fatalf("expected apt-get command output, got %+v", plan.Command)
	}

	plan, err = store.RenderAction(PackageManagerActionInput{
		OS:        "windows",
		Preferred: "winget",
		Action:    "hold",
		Package:   "Git.Git",
	})
	if err != nil {
		t.Fatalf("render unsupported hold action failed unexpectedly: %v", err)
	}
	if plan.Allowed || plan.BlockedReason == "" {
		t.Fatalf("expected blocked hold action for winget, got %+v", plan)
	}
}
