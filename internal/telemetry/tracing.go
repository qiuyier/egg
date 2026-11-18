package telemetry

import (
	"context"
	"fmt"

	"github.com/qiuyier/egg/internal/config"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

func New(ctx context.Context, app config.App, cfg config.Tracing) (*sdktrace.TracerProvider, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceName(app.Name),
			semconv.DeploymentEnvironmentNameKey.String(app.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("initialize tracing resource: %w", err)
	}

	var options []otlptracehttp.Option
	if cfg.Endpoint != "" {
		options = append(options, otlptracehttp.WithEndpointURL(cfg.Endpoint))
	}
	exporter, err := otlptracehttp.New(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("initialize OTLP/HTTP exporter: %w", err)
	}
	return sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRate))),
		sdktrace.WithBatcher(exporter),
	), nil
}

func Propagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
}
