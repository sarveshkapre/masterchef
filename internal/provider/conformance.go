package provider

import (
	"context"
	"fmt"

	"github.com/masterchef/masterchef/internal/config"
)

type ConformanceReport struct {
	ProviderType   string `json:"provider_type"`
	IdempotentPass bool   `json:"idempotent_pass"`
	Error          string `json:"error,omitempty"`
}

// CheckIdempotency runs the same apply twice and expects changed=false on second run.
func CheckIdempotency(ctx context.Context, h Handler, sample config.Resource) ConformanceReport {
	rep := ConformanceReport{
		ProviderType: h.Type(),
	}
	_, err := h.Apply(ctx, sample)
	if err != nil {
		rep.Error = fmt.Sprintf("first apply failed: %v", err)
		return rep
	}
	second, err := h.Apply(ctx, sample)
	if err != nil {
		rep.Error = fmt.Sprintf("second apply failed: %v", err)
		return rep
	}
	if second.Changed {
		rep.Error = "second apply unexpectedly changed resource"
		return rep
	}
	rep.IdempotentPass = true
	return rep
}
