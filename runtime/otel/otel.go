//go:build otel

package otel

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	tracer         trace.Tracer
)

func init() {
	Enabled = true
}

// Init sets up the OTel SDK: TracerProvider, MeterProvider, and text map propagator.
// Reads standard OTEL_* environment variables.
func Init() {
	ctx := context.Background()

	// Resource
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName()),
			semconv.ProcessRuntimeName("gun"),
			semconv.TelemetrySDKName("gun-opentelemetry"),
			semconv.TelemetrySDKLanguageGo,
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[otel] resource creation failed: %v\n", err)
		return
	}

	protocol := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	if protocol == "" {
		protocol = "http/protobuf"
	}

	// Tracer provider
	tp, err := initTracerProvider(ctx, res, protocol)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[otel] tracer provider init failed: %v\n", err)
	} else {
		tracerProvider = tp
		otel.SetTracerProvider(tp)
	}

	// Meter provider
	mp, err := initMeterProvider(ctx, res, protocol)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[otel] meter provider init failed: %v\n", err)
	} else {
		meterProvider = mp
		otel.SetMeterProvider(mp)
	}

	// Propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer = otel.GetTracerProvider().Tracer("gun")
	initMetrics()
}

// Shutdown flushes pending telemetry and shuts down providers.
func Shutdown() {
	if tracerProvider != nil {
		_ = tracerProvider.Shutdown(context.Background())
	}
	if meterProvider != nil {
		_ = meterProvider.Shutdown(context.Background())
	}
}

func serviceName() string {
	if name := os.Getenv("OTEL_SERVICE_NAME"); name != "" {
		return name
	}
	return "gun-app"
}

func initTracerProvider(ctx context.Context, res *resource.Resource, protocol string) (*sdktrace.TracerProvider, error) {
	var exporter sdktrace.SpanExporter
	var err error

	switch protocol {
	case "console":
		exporter, err = stdouttrace.New(stdouttrace.WithWriter(os.Stderr))
	default:
		opts := []otlptracehttp.Option{}
		if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
			opts = append(opts, otlptracehttp.WithEndpoint(endpoint))
		}
		opts = append(opts, otlptracehttp.WithInsecure())
		exporter, err = otlptracehttp.New(ctx, opts...)
	}
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
	), nil
}

func initMeterProvider(ctx context.Context, res *resource.Resource, protocol string) (*sdkmetric.MeterProvider, error) {
	var exporter sdkmetric.Exporter
	var err error

	switch protocol {
	case "console":
		exporter, err = stdoutmetric.New(stdoutmetric.WithWriter(os.Stderr))
	default:
		opts := []otlpmetrichttp.Option{}
		if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
			opts = append(opts, otlpmetrichttp.WithEndpoint(strings.TrimSuffix(endpoint, "/")))
		}
		opts = append(opts, otlpmetrichttp.WithInsecure())
		exporter, err = otlpmetrichttp.New(ctx, opts...)
	}
	if err != nil {
		return nil, fmt.Errorf("create metric exporter: %w", err)
	}

	return sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
	), nil
}
