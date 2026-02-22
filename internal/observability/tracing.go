package observability

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func InitTracing(ctx context.Context) (func(context.Context) error, error) {

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(
		ctx,
		resource.WithFromEnv(),
		resource.WithAttributes(
			semconv.ServiceName(ServiceName()),
		),
	)
	if err != nil {
		return nil, err
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(provider)

	return provider.Shutdown, nil
}

func ServiceName() string {
	name := os.Getenv("OTEL_SERVICE_NAME")
	if name == "" {
		name = "go-chi-api"
	}
	return name
}
