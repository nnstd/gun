//go:build otel

package otel

import (
	"go.opentelemetry.io/otel/trace"
)

// SpanContext returns the trace and span IDs from the active span context.
// Returns empty strings when no span is active.
func SpanContext() (traceID, spanID string) {
	sc := trace.SpanFromContext(ActiveContext()).SpanContext()
	if !sc.IsValid() {
		return "", ""
	}
	return sc.TraceID().String(), sc.SpanID().String()
}
