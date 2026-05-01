//go:build otel

package otel

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	metricRequestDuration  metric.Float64Histogram
	metricActiveRequests   metric.Int64UpDownCounter
	metricRequestBodySize  metric.Int64Histogram
	metricResponseBodySize metric.Int64Histogram
)

func initMetrics() {
	m := meterProvider.Meter("gun")

	var err error

	metricRequestDuration, err = m.Float64Histogram("http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of incoming HTTP requests"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1.0, 2.5, 5.0, 7.5, 10.0),
	)
	if err != nil {
		panic(err)
	}

	metricActiveRequests, err = m.Int64UpDownCounter("http.server.active_requests",
		metric.WithUnit("1"),
		metric.WithDescription("Number of active HTTP server requests"),
	)
	if err != nil {
		panic(err)
	}

	metricRequestBodySize, err = m.Int64Histogram("http.server.request.body.size",
		metric.WithUnit("By"),
		metric.WithDescription("Size of HTTP request bodies"),
		metric.WithExplicitBucketBoundaries(0, 100, 1000, 10000, 100000, 1000000, 10000000, 100000000, 1000000000),
	)
	if err != nil {
		panic(err)
	}

	metricResponseBodySize, err = m.Int64Histogram("http.server.response.body.size",
		metric.WithUnit("By"),
		metric.WithDescription("Size of HTTP response bodies"),
		metric.WithExplicitBucketBoundaries(0, 100, 1000, 10000, 100000, 1000000, 10000000, 100000000, 1000000000),
	)
	if err != nil {
		panic(err)
	}
}

// RecordActiveRequest increments or decrements the active request counter.
func RecordActiveRequest(method, scheme, addr string, port int, delta int64) {
	metricActiveRequests.Add(context.Background(), delta,
		metric.WithAttributes(
			attribute.String("http.request.method", method),
			attribute.String("url.scheme", scheme),
			attribute.String("server.address", addr),
			attribute.Int("server.port", port),
		),
	)
}

// RecordServerRequest records metrics for a completed HTTP request.
func RecordServerRequest(method, scheme, protoVersion, addr string, port, statusCode int, errType string, duration time.Duration, reqBodySize, resBodySize int64) {
	attrs := []attribute.KeyValue{
		attribute.String("http.request.method", method),
		attribute.String("url.scheme", scheme),
		attribute.String("network.protocol.version", protoVersion),
		attribute.String("server.address", addr),
		attribute.Int("server.port", port),
	}
	if statusCode > 0 {
		attrs = append(attrs, attribute.Int("http.response.status_code", statusCode))
	}
	if errType != "" {
		attrs = append(attrs, attribute.String("error.type", errType))
	}

	opt := metric.WithAttributes(attrs...)

	metricRequestDuration.Record(context.Background(), duration.Seconds(), opt)
	metricRequestBodySize.Record(context.Background(), reqBodySize, opt)
	metricResponseBodySize.Record(context.Background(), resBodySize, opt)
}
