---
title: OpenTelemetry
lead: Enable distributed tracing and HTTP metrics with a single flag. No code changes required.
sections:
  - Quick start
  - Auto-instrumented metrics
  - Auto-instrumented spans
  - Console output
  - Configuration
  - Exporting to a collector
---

## Quick start

Pass `--otel` to any run or build command:

```bash
gun run server.ts --otel
```

That is all. Gun initializes the OpenTelemetry SDK at startup, instruments your HTTP server and fetch calls, and flushes telemetry on shutdown.

When `--otel` is not passed, zero OTel code is compiled into the binary. There is no runtime overhead.

## Auto-instrumented metrics

Gun emits four HTTP server metrics matching the [OpenTelemetry HTTP semantic conventions](https://opentelemetry.io/docs/specs/semconv/http/http-metrics/):

| Metric | Type | Description |
|--------|------|-------------|
| `http.server.request.duration` | Histogram | Request latency in seconds |
| `http.server.active_requests` | UpDownCounter | Concurrent in-flight requests |
| `http.server.request.body.size` | Histogram | Request body size in bytes |
| `http.server.response.body.size` | Histogram | Response body size in bytes |

Duration histogram buckets (seconds):

```
0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10
```

Body size histogram buckets (bytes):

```
0, 100, 1K, 10K, 100K, 1M, 10M, 100M, 1B
```

All metrics include attributes: `http.request.method`, `url.scheme`, `server.address`, `server.port`, `http.response.status_code`, and `error.type` when applicable.

## Auto-instrumented spans

### Server spans (`Bun.serve`, `http.createServer`)

Every incoming HTTP request creates a server span with attributes:

- `http.request.method`
- `url.full`, `url.scheme`, `url.path`, `url.query`
- `http.response.status_code` (set on completion)

### Client spans (`fetch`)

Every outgoing `fetch` call creates a client span with the same URL and method attributes.

Parent-child linkage is preserved: a `fetch` inside a request handler automatically becomes a child of the server span.

## Console output

When OTel is enabled, `console.log` / `console.error` / `console.warn` prepend the trace and span ID:

```
[4bf92f3577b34da6a3ce929d0e0e4736/00f067aa0ba902b7] Server started on port 3000
```

This lets you correlate logs with traces in any OTel-compatible backend.

## Configuration

Gun reads standard OTel environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4318` | OTLP collector endpoint |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` | `http/protobuf` or `console` |
| `OTEL_SERVICE_NAME` | `gun-app` | Service name in resource attributes |

When no endpoint is configured, telemetry defaults to stdout (console exporter) for easy local debugging.

## Exporting to a collector

Point to any OTLP-compatible collector (Jaeger, Grafana Tempo, Honeycomb, Datadog):

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318 \
OTEL_SERVICE_NAME=my-api \
gun run server.ts --otel
```

Or use the console exporter for local development:

```bash
OTEL_EXPORTER_OTLP_PROTOCOL=console gun run server.ts --otel
```
