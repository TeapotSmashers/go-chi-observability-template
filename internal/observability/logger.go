package observability

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var Logger *zap.Logger

func InitLogger() error {
	var err error

	Logger, err = zap.NewProduction()
	if err != nil {
		return err
	}

	return nil
}

func SyncLogger() {
	_ = Logger.Sync()
}

func LoggerWithTrace(ctx context.Context) *zap.Logger {
	span := trace.SpanContextFromContext(ctx)

	if !span.IsValid() {
		return Logger
	}

	return Logger.With(
		zap.String("trace_id", span.TraceID().String()),
		zap.String("span_id", span.SpanID().String()),
	)
}
