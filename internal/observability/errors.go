package observability

import (
	"context"
	"encoding/json"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// RecordError centralises error handling across all domains: records the error
// on the span, increments the provided error counter, logs with trace context,
// and writes a JSON error HTTP response.
func RecordError(ctx context.Context, span trace.Span, logger *zap.Logger, counter metric.Int64Counter, opName, msg string, err error, status int, w http.ResponseWriter) {
	span.RecordError(err)
	span.SetStatus(codes.Error, msg)

	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", opName)))

	logger.Error(msg,
		zap.String("operation", opName),
		zap.Error(err),
		zap.String("request_id", RequestIDFromContext(ctx)),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":      msg,
		"request_id": RequestIDFromContext(ctx),
	})
}
