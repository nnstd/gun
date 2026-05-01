//go:build otel

package otel

import (
	"context"
	"net/url"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// StartHTTPSpan creates a server span for an incoming HTTP request.
func StartHTTPSpan(ctx context.Context, method, fullURL, scheme, path, query string) (context.Context, trace.Span) {
	ctx, span := tracer.Start(ctx, method,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("http.request.method", method),
			attribute.String("url.full", fullURL),
			attribute.String("url.scheme", scheme),
			attribute.String("url.path", path),
			attribute.String("url.query", query),
		),
	)
	return ctx, span
}

// EndHTTPSpan sets status code and ends the span.
func EndHTTPSpan(span interface{}, statusCode int) {
	if span == nil {
		return
	}
	s, ok := span.(trace.Span)
	if !ok {
		return
	}
	s.SetAttributes(attribute.Int("http.response.status_code", statusCode))
	if statusCode >= 500 {
		s.SetStatus(codes.Error, "")
	}
	s.End()
}

// StartFetchSpan creates a client span for an outgoing fetch request.
func StartFetchSpan(ctx context.Context, method, fullURL string) (context.Context, interface{}) {
	u, _ := url.Parse(fullURL)
	path := "/"
	query := ""
	if u != nil {
		path = u.Path
		query = u.RawQuery
	}
	scheme := "http"
	if u != nil && u.Scheme != "" {
		scheme = u.Scheme
	}

	_, span := tracer.Start(ctx, method,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("http.request.method", method),
			attribute.String("url.full", fullURL),
			attribute.String("url.scheme", scheme),
			attribute.String("url.path", path),
			attribute.String("url.query", query),
		),
	)
	return ctx, span
}
