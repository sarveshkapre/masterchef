package server

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/masterchef/masterchef/internal/state"
)

type runCorrelation struct {
	StepOrder     int               `json:"step_order"`
	ResourceID    string            `json:"resource_id"`
	ResourceType  string            `json:"resource_type"`
	Host          string            `json:"host"`
	Changed       bool              `json:"changed"`
	Skipped       bool              `json:"skipped"`
	CorrelationID string            `json:"correlation_id"`
	TraceID       string            `json:"trace_id"`
	SpanID        string            `json:"span_id"`
	Observability map[string]string `json:"observability"`
}

func buildRunCorrelations(run state.RunRecord) []runCorrelation {
	out := make([]runCorrelation, 0, len(run.Results))
	for i, res := range run.Results {
		seed := strings.Join([]string{
			run.ID,
			strconv.Itoa(i + 1),
			res.ResourceID,
			res.Type,
			res.Host,
			res.Message,
		}, "|")
		sum := sha256.Sum256([]byte(seed))
		h := hex.EncodeToString(sum[:])
		traceID := h[:32]
		spanID := h[32:48]
		correlationID := "corr-" + h[:16]
		out = append(out, runCorrelation{
			StepOrder:     i + 1,
			ResourceID:    res.ResourceID,
			ResourceType:  res.Type,
			Host:          res.Host,
			Changed:       res.Changed,
			Skipped:       res.Skipped,
			CorrelationID: correlationID,
			TraceID:       traceID,
			SpanID:        spanID,
			Observability: map[string]string{
				"trace_url": "otel://trace/" + traceID,
				"span_url":  "otel://span/" + spanID,
			},
		})
	}
	return out
}
