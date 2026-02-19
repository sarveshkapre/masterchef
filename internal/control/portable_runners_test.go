package control

import "testing"

func TestPortableRunnerCatalogSelectBuiltin(t *testing.T) {
	catalog := NewPortableRunnerCatalog()
	result, err := catalog.Select(PortableRunnerSelectRequest{OSFamily: "linux", Architecture: "amd64", TransportMode: "ssh"})
	if err != nil {
		t.Fatalf("select runner: %v", err)
	}
	if !result.Supported {
		t.Fatalf("expected supported result: %+v", result)
	}
	if result.Profile.Name != "posix-static-runner" {
		t.Fatalf("unexpected selected profile: %+v", result.Profile)
	}
}

func TestPortableRunnerCatalogRegisterAndSelect(t *testing.T) {
	catalog := NewPortableRunnerCatalog()
	item, err := catalog.Register(PortableRunnerProfileInput{
		Name:             "linux-s390x-runner",
		OSFamilies:       []string{"linux"},
		Architectures:    []string{"s390x"},
		TransportModes:   []string{"ssh"},
		Shell:            "sh",
		ArtifactRef:      "runner://linux-s390x",
		ChecksumSHA256:   "sha256:3333333333333333333333333333333333333333333333333333333333333333",
		SupportsNoPython: true,
	})
	if err != nil {
		t.Fatalf("register runner: %v", err)
	}
	if item.ID == "" || item.Builtin {
		t.Fatalf("unexpected registered profile: %+v", item)
	}
	result, err := catalog.Select(PortableRunnerSelectRequest{OSFamily: "linux", Architecture: "s390x", TransportMode: "ssh"})
	if err != nil {
		t.Fatalf("select custom runner: %v", err)
	}
	if !result.Supported || result.Profile.ID != item.ID {
		t.Fatalf("expected custom runner selection, got %+v", result)
	}
}

func TestPortableRunnerCatalogSelectPythonRequiredNoMatch(t *testing.T) {
	catalog := NewPortableRunnerCatalog()
	result, err := catalog.Select(PortableRunnerSelectRequest{OSFamily: "linux", Architecture: "amd64", TransportMode: "ssh", PythonRequired: true})
	if err != nil {
		t.Fatalf("select with python required: %v", err)
	}
	if result.Supported {
		t.Fatalf("expected unsupported when python required and no python profile exists")
	}
}
