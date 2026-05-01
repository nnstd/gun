//go:build !otel

package otel

import (
	"context"
	"time"
)

// No-op stubs when OTel is not enabled via build tag.
// Real implementations live in otel.go.

func init() {
	Enabled = false
}

func Init() {}

func Shutdown() {}

func StartFetchSpan(ctx context.Context, method, fullURL string) (context.Context, interface{}) {
	return ctx, nil
}

func StartHTTPSpan(ctx context.Context, method, fullURL, scheme, path, query string) (context.Context, interface{}) {
	return ctx, nil
}

func EndHTTPSpan(span interface{}, statusCode int) {}

func RecordActiveRequest(method, scheme, addr string, port int, delta int64) {}

func RecordServerRequest(method, scheme, protoVersion, addr string, port, statusCode int, errType string, duration time.Duration, reqBodySize, resBodySize int64) {}

func SpanContext() (traceID, spanID string) { return "", "" }
