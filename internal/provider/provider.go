package provider

import (
	"context"
	"fmt"

	"github.com/masterchef/masterchef/internal/config"
)

type Result struct {
	Changed bool
	Skipped bool
	Message string
}

type Handler interface {
	Type() string
	Apply(ctx context.Context, resource config.Resource) (Result, error)
}

type Registry struct {
	handlers map[string]Handler
}

func NewRegistry() *Registry {
	return &Registry{
		handlers: map[string]Handler{},
	}
}

func (r *Registry) Register(h Handler) error {
	if h == nil {
		return fmt.Errorf("handler is nil")
	}
	t := h.Type()
	if t == "" {
		return fmt.Errorf("handler type is empty")
	}
	if _, exists := r.handlers[t]; exists {
		return fmt.Errorf("handler already registered for type %q", t)
	}
	r.handlers[t] = h
	return nil
}

func (r *Registry) MustRegister(h Handler) {
	if err := r.Register(h); err != nil {
		panic(err)
	}
}

func (r *Registry) Lookup(resourceType string) (Handler, bool) {
	h, ok := r.handlers[resourceType]
	return h, ok
}
