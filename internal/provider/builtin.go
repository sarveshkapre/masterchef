package provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/masterchef/masterchef/internal/config"
)

type FileHandler struct{}

func (h *FileHandler) Type() string { return "file" }

func (h *FileHandler) Apply(_ context.Context, resource config.Resource) (Result, error) {
	full := filepath.Clean(resource.Path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return Result{}, fmt.Errorf("mkdir for file resource: %w", err)
	}

	current, err := os.ReadFile(full)
	if err == nil && string(current) == resource.Content {
		return Result{Changed: false, Message: "file already in desired state"}, nil
	}
	if err := os.WriteFile(full, []byte(resource.Content), 0o644); err != nil {
		return Result{}, fmt.Errorf("write file: %w", err)
	}
	return Result{Changed: true, Message: "file updated"}, nil
}

type CommandHandler struct{}

func (h *CommandHandler) Type() string { return "command" }

func (h *CommandHandler) Apply(_ context.Context, resource config.Resource) (Result, error) {
	if resource.Creates != "" {
		if _, err := os.Stat(resource.Creates); err == nil {
			return Result{Skipped: true, Message: "command skipped: creates path already exists"}, nil
		}
	}
	if resource.Unless != "" {
		if err := exec.Command("sh", "-c", resource.Unless).Run(); err == nil {
			return Result{Skipped: true, Message: "command skipped: unless condition succeeded"}, nil
		}
	}

	cmd := exec.Command("sh", "-c", resource.Command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{}, fmt.Errorf("command failed: %w: %s", err, string(out))
	}
	return Result{Changed: true, Message: string(out)}, nil
}

func NewBuiltinRegistry() *Registry {
	r := NewRegistry()
	r.MustRegister(&FileHandler{})
	r.MustRegister(&CommandHandler{})
	return r
}
