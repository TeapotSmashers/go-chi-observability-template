package observability

import (
	"context"

	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func InitLogging(ctx context.Context) (func(context.Context) error, error) {

	exporter, err := otlploghttp.New(ctx)
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

	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(
			sdklog.NewBatchProcessor(exporter),
		),
	)

	otelCore := otelzap.NewCore(ServiceName(), otelzap.WithLoggerProvider(provider))

	// Tee the existing stdout logger core with the OTel core so logs
	// go to both stdout and the OTLP endpoint.
	Logger = zap.New(zapcore.NewTee(Logger.Core(), otelCore))

	return provider.Shutdown, nil
}
