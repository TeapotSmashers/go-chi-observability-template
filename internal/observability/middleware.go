package observability

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"
)

var untracedPaths = map[string]struct{}{
	"/metrics": {},
	"/health":  {},
}

func shouldTraceRequest(r *http.Request) bool {
	_, skip := untracedPaths[r.URL.Path]
	return !skip
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		requestID := NewRequestID()
		ctx := ContextWithRequestID(r.Context(), requestID)

		w.Header().Set("X-Request-ID", requestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoggingMiddleware(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		start := time.Now()

		ctx := r.Context()
		logger := LoggerWithTrace(ctx)

		next.ServeHTTP(w, r)

		logger.Info("request completed",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("request_id", RequestIDFromContext(ctx)),
			zap.Duration("duration", time.Since(start)),
		)
	})
}

func TracingMiddleware(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "http_request", otelhttp.WithFilter(shouldTraceRequest))
}
