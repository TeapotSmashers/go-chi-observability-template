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

// LoggerWithTrace returns a child logger enriched with trace_id and span_id
// fields from the active OTel span in ctx.
//
// It also embeds ctx itself as a zap.Any("context", ctx) field. The otelzap
// bridge's Write method (core.go:convertField) detects any field whose
// Interface value implements context.Context and uses it as the context passed
// to log.Logger.Emit. This causes the OTel SDK to populate the native TraceID
// and SpanID fields on the outgoing OTLP log record — which is what Loki
// stores as structured metadata under "traceID", enabling proper Loki → Tempo
// trace correlation via the Grafana derived field.
//
// Without this, the otelzap bridge calls Emit with context.Background(), so
// the native OTel TraceID on every log record is all-zeros, and the only
// trace_id present is a plain string attribute — which Loki stores as
// structured metadata (not an index label), breaking the derived field lookup.
//
// The human-readable trace_id / span_id string fields are kept so that stdout
// JSON logs remain greppable without an OTel-aware tool.
func LoggerWithTrace(ctx context.Context) *zap.Logger {
	span := trace.SpanContextFromContext(ctx)

	if !span.IsValid() {
		return Logger
	}

	return Logger.With(
		// Picked up by otelzap.Core.Write → convertField, which sets the
		// context used in log.Logger.Emit, populating the native OTel
		// TraceID/SpanID on the exported OTLP log record.
		zap.Any("context", ctx),
		// Human-readable fields for stdout JSON and ad-hoc log grepping.
		zap.String("trace_id", span.TraceID().String()),
		zap.String("span_id", span.SpanID().String()),
	)
}
