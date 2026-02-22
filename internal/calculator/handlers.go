package calculator

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"go-chi-observability/internal/handlers"
	"go-chi-observability/internal/observability"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// tracer is the calculator's dedicated OpenTelemetry tracer.
var tracer = otel.Tracer("calculator")

// ---------------------------------------------------------------------------
// Handlers — binary operations
// ---------------------------------------------------------------------------

// Add handles POST /calculator/add
func Add(w http.ResponseWriter, r *http.Request) {
	handleBinaryOp(w, r, "add", func(a, b float64) (float64, error) {
		return a + b, nil
	})
}

// Subtract handles POST /calculator/subtract
func Subtract(w http.ResponseWriter, r *http.Request) {
	handleBinaryOp(w, r, "subtract", func(a, b float64) (float64, error) {
		return a - b, nil
	})
}

// Multiply handles POST /calculator/multiply
func Multiply(w http.ResponseWriter, r *http.Request) {
	handleBinaryOp(w, r, "multiply", func(a, b float64) (float64, error) {
		return a * b, nil
	})
}

// Divide handles POST /calculator/divide — demonstrates error recording on spans.
func Divide(w http.ResponseWriter, r *http.Request) {
	handleBinaryOp(w, r, "divide", func(a, b float64) (float64, error) {
		if b == 0 {
			return 0, fmt.Errorf("division by zero: %g / %g", a, b)
		}
		return a / b, nil
	})
}

// handleBinaryOp is the shared implementation for all binary calculator operations.
// It demonstrates: custom child spans, span attributes & events, custom metrics,
// trace-correlated structured logging, error recording, and request-ID propagation.
func handleBinaryOp(w http.ResponseWriter, r *http.Request, opName string, compute func(float64, float64) (float64, error)) {
	ctx := r.Context()
	logger := observability.LoggerWithTrace(ctx)
	requestID := observability.RequestIDFromContext(ctx)

	// --- 1. Custom child span ---
	ctx, span := tracer.Start(ctx, fmt.Sprintf("calculator.%s", opName),
		trace.WithAttributes(
			attribute.String("calculator.operation", opName),
			attribute.String("request.id", requestID),
		),
	)
	defer span.End()

	// --- 2. Decode request body ---
	var req CalcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		observability.RecordError(ctx, span, logger, errorCounter, opName, "invalid request body", err, http.StatusBadRequest, w)
		return
	}

	// Validate inputs
	if math.IsNaN(req.A) || math.IsInf(req.A, 0) || math.IsNaN(req.B) || math.IsInf(req.B, 0) {
		observability.RecordError(ctx, span, logger, errorCounter, opName, "invalid numeric input", fmt.Errorf("a=%g b=%g", req.A, req.B), http.StatusBadRequest, w)
		return
	}

	// Record operands as span attributes
	span.SetAttributes(
		attribute.Float64("calculator.operand.a", req.A),
		attribute.Float64("calculator.operand.b", req.B),
	)

	// --- 3. Perform computation (timed for histogram) ---
	start := time.Now()
	result, err := compute(req.A, req.B)
	elapsed := float64(time.Since(start).Microseconds()) / 1000.0 // ms

	if err != nil {
		observability.RecordError(ctx, span, logger, errorCounter, opName, err.Error(), err, http.StatusBadRequest, w)
		return
	}

	// --- 4. Record metrics ---
	attrs := metric.WithAttributes(attribute.String("operation", opName))
	opsCounter.Add(ctx, 1, attrs)
	opsHistogram.Record(ctx, elapsed, attrs)
	resultGauge.Record(ctx, result, attrs)

	// --- 5. Span event with the result ---
	span.AddEvent("computation.complete", trace.WithAttributes(
		attribute.Float64("result", result),
		attribute.Float64("duration_ms", elapsed),
	))
	span.SetAttributes(attribute.Float64("calculator.result", result))
	span.SetStatus(codes.Ok, "")

	// --- 6. Structured log with trace correlation ---
	logger.Info("calculator operation completed",
		zap.String("operation", opName),
		zap.Float64("a", req.A),
		zap.Float64("b", req.B),
		zap.Float64("result", result),
		zap.String("request_id", requestID),
		zap.Float64("duration_ms", elapsed),
	)

	// --- 7. Write JSON response ---
	resp := CalcResponse{
		Operation: opName,
		A:         req.A,
		B:         req.B,
		Result:    result,
		RequestID: requestID,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// ---------------------------------------------------------------------------
// Handler — chained operations (demonstrates nested spans)
// ---------------------------------------------------------------------------

// Chain handles POST /calculator/chain — runs a sequence of operations on a
// running total, creating a child span for every step. This produces a
// multi-level trace that is ideal for visualising in Jaeger / Grafana Tempo.
func Chain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := observability.LoggerWithTrace(ctx)
	requestID := observability.RequestIDFromContext(ctx)

	// Parent span for the entire chain
	ctx, span := tracer.Start(ctx, "calculator.chain",
		trace.WithAttributes(
			attribute.String("request.id", requestID),
		),
	)
	defer span.End()

	// Decode
	var req ChainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		observability.RecordError(ctx, span, logger, errorCounter, "chain", "invalid request body", err, http.StatusBadRequest, w)
		return
	}

	if len(req.Steps) == 0 {
		observability.RecordError(ctx, span, logger, errorCounter, "chain", "no steps provided", fmt.Errorf("steps array is empty"), http.StatusBadRequest, w)
		return
	}

	span.SetAttributes(
		attribute.Float64("chain.initial", req.Initial),
		attribute.Int("chain.steps_count", len(req.Steps)),
	)

	logger.Info("starting chained calculation",
		zap.Float64("initial", req.Initial),
		zap.Int("steps", len(req.Steps)),
		zap.String("request_id", requestID),
	)

	running := req.Initial
	results := make([]ChainResult, 0, len(req.Steps))

	for i, step := range req.Steps {
		// --- Child span per step ---
		_, stepSpan := tracer.Start(ctx, fmt.Sprintf("calculator.chain.step.%d.%s", i, step.Op),
			trace.WithAttributes(
				attribute.Int("chain.step.index", i),
				attribute.String("chain.step.operation", step.Op),
				attribute.Float64("chain.step.input", running),
				attribute.Float64("chain.step.value", step.Value),
			),
		)

		stepStart := time.Now()
		var err error
		prev := running

		switch step.Op {
		case "add":
			running += step.Value
		case "subtract":
			running -= step.Value
		case "multiply":
			running *= step.Value
		case "divide":
			if step.Value == 0 {
				err = fmt.Errorf("division by zero at step %d", i)
			} else {
				running /= step.Value
			}
		default:
			err = fmt.Errorf("unknown operation %q at step %d", step.Op, i)
		}

		stepElapsed := float64(time.Since(stepStart).Microseconds()) / 1000.0

		if err != nil {
			// Record error on the child step span
			stepSpan.RecordError(err)
			stepSpan.SetStatus(codes.Error, err.Error())
			stepSpan.End()

			// Record error on the parent chain span
			span.RecordError(err)
			span.SetStatus(codes.Error, fmt.Sprintf("failed at step %d", i))

			// Metric + log + HTTP response
			errorCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", step.Op)))

			logger.Error("chain step failed",
				zap.Int("step", i),
				zap.String("operation", step.Op),
				zap.Error(err),
				zap.String("request_id", requestID),
			)

			handlers.WriteError(w, http.StatusBadRequest, err.Error(), requestID)
			return
		}

		// Record step metrics
		attrs := metric.WithAttributes(attribute.String("operation", step.Op))
		opsCounter.Add(ctx, 1, attrs)
		opsHistogram.Record(ctx, stepElapsed, attrs)

		stepSpan.AddEvent("step.complete", trace.WithAttributes(
			attribute.Float64("input", prev),
			attribute.Float64("result", running),
		))
		stepSpan.SetAttributes(attribute.Float64("chain.step.result", running))
		stepSpan.SetStatus(codes.Ok, "")
		stepSpan.End()

		logger.Info("chain step completed",
			zap.Int("step", i),
			zap.String("operation", step.Op),
			zap.Float64("input", prev),
			zap.Float64("value", step.Value),
			zap.Float64("result", running),
			zap.Float64("duration_ms", stepElapsed),
		)

		results = append(results, ChainResult{
			Op:     step.Op,
			Value:  step.Value,
			Result: running,
		})
	}

	// Record final result
	resultGauge.Record(ctx, running, metric.WithAttributes(attribute.String("operation", "chain")))

	span.AddEvent("chain.complete", trace.WithAttributes(
		attribute.Float64("final_result", running),
		attribute.Int("total_steps", len(req.Steps)),
	))
	span.SetAttributes(attribute.Float64("chain.result", running))
	span.SetStatus(codes.Ok, "")

	logger.Info("chained calculation completed",
		zap.Float64("initial", req.Initial),
		zap.Float64("result", running),
		zap.Int("steps", len(req.Steps)),
		zap.String("request_id", requestID),
	)

	resp := ChainResponse{
		Initial:   req.Initial,
		Steps:     results,
		Result:    running,
		RequestID: requestID,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}
