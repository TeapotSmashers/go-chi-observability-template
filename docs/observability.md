# Observability Infrastructure

This document covers the internal implementation of the observability stack and — most importantly — how to correctly instrument new functionality.

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [The Three Pillars](#the-three-pillars)
  - [1. Structured Logging](#1-structured-logging)
  - [2. Distributed Tracing](#2-distributed-tracing)
  - [3. Metrics](#3-metrics)
- [Request ID](#request-id)
- [Middleware Stack](#middleware-stack)
- [Shared Error Handling](#shared-error-handling)
- [Instrumenting New Functionality](#instrumenting-new-functionality)
  - [Step-by-step Checklist](#step-by-step-checklist)
  - [Creating Custom Spans](#creating-custom-spans)
  - [Defining Domain Metrics](#defining-domain-metrics)
  - [Logging Correctly](#logging-correctly)
  - [Handling Errors](#handling-errors)
  - [Nested Spans](#nested-spans)
- [Environment Variables](#environment-variables)

---

## Architecture Overview

The observability layer lives entirely in `internal/observability/`. It is a **generic infrastructure package** that has no knowledge of application domains. Domain packages (like `internal/calculator/`) import it — never the reverse.

```
cmd/api/main.go            # Initialises logger -> tracing -> metrics (in order)
cmd/api/init.go            # Wires domain-specific metric instruments

internal/observability/
  logger.go                # Zap structured logger + trace correlation
  tracing.go               # OTel TracerProvider (OTLP/HTTP exporter)
  metrics.go               # OTel MeterProvider (OTLP/HTTP) + Prometheus /metrics
  middleware.go            # RequestID, Tracing, Logging middlewares
  request_id.go            # UUID-based request ID with context propagation
  errors.go                # RecordError — shared span+metric+log+response helper
```

### Initialisation Order

The order matters because later systems depend on earlier ones:

```
1. InitLogger()      — Zap logger available globally
2. InitTracing(ctx)  — OTel TracerProvider registered, spans can be created
3. initMetrics(ctx)  — OTel MeterProvider registered, then domain metrics init
4. NewRouter()       — Middleware stack wired, routes registered
```

All init functions return shutdown closures that are deferred in `main()` for graceful drain on `SIGINT`/`SIGTERM`.

---

## The Three Pillars

### 1. Structured Logging

**File:** `internal/observability/logger.go`
**Library:** `go.uber.org/zap` (production JSON encoder)

| Symbol | Purpose |
|---|---|
| `Logger` | Package-level `*zap.Logger` — initialised once by `InitLogger()` |
| `SyncLogger()` | Flushes buffered log entries (deferred in `main()`) |
| `LoggerWithTrace(ctx)` | Returns a child logger enriched with `trace_id` and `span_id` extracted from the OTel span context in the Go context |

**Key design decision:** `LoggerWithTrace` bridges Zap and OpenTelemetry. Every log line emitted through it automatically carries trace/span IDs, enabling log-to-trace correlation in backends like Grafana, Datadog, or Elastic.

```go
// In any handler or function with a context:
logger := observability.LoggerWithTrace(ctx)
logger.Info("something happened",
    zap.String("key", "value"),
)
// Output includes trace_id and span_id automatically
```

If no valid span exists in the context (e.g. during startup), it returns the base `Logger` unchanged — no panic, no nil pointer.

### 2. Distributed Tracing

**File:** `internal/observability/tracing.go`
**Library:** OpenTelemetry SDK (`go.opentelemetry.io/otel/sdk/trace`)
**Exporter:** OTLP over HTTP (`otlptracehttp`)

`InitTracing(ctx)` performs the following:

1. Creates an OTLP/HTTP exporter (configured entirely via `OTEL_*` environment variables)
2. Builds a `Resource` from environment attributes (`OTEL_RESOURCE_ATTRIBUTES`)
3. Creates a `TracerProvider` with batched export (`WithBatcher`)
4. Registers it globally via `otel.SetTracerProvider(provider)`
5. Returns `provider.Shutdown` for graceful drain of in-flight spans

**Automatic instrumentation** is provided by the `TracingMiddleware` (see [Middleware Stack](#middleware-stack)). It wraps every incoming HTTP request in a span via `otelhttp.NewHandler`, which automatically records:
- HTTP method, URL, status code
- Request/response size
- Duration
- W3C `traceparent`/`tracestate` header propagation

**Manual instrumentation** is done in handlers by creating child spans with `tracer.Start()`. See [Creating Custom Spans](#creating-custom-spans).

### 3. Metrics

**File:** `internal/observability/metrics.go`
**Libraries:** OpenTelemetry SDK + Prometheus client

Two independent metric systems coexist:

| System | Transport | Endpoint | Purpose |
|---|---|---|---|
| OTel Metrics | OTLP/HTTP push | Configured via `OTEL_*` env vars | Push metrics to a collector (Grafana Cloud, Datadog, etc.) |
| Prometheus | HTTP pull | `GET /metrics` | Standard Prometheus scrape endpoint with Go runtime metrics |

**These two systems are not bridged.** OTel metrics are pushed via the `PeriodicReader`; Prometheus metrics are independently scraped. Custom application metrics (counters, histograms, gauges) are registered through OTel and pushed via OTLP. The Prometheus endpoint exposes Go runtime metrics out of the box.

Domain-specific metric instruments are defined in each domain's `metrics.go` file (e.g. `internal/calculator/metrics.go`) and initialised via `InitMetrics()`, which is called from `cmd/api/init.go`.

---

## Request ID

**File:** `internal/observability/request_id.go`

Every incoming request gets a UUID v4 request ID. The pattern uses Go's context-value propagation with an unexported key type to prevent collisions:

```go
type contextKey string
const RequestIDKey contextKey = "request_id"
```

| Function | Purpose |
|---|---|
| `NewRequestID()` | Generates a UUID v4 string |
| `ContextWithRequestID(ctx, id)` | Stores the ID in the context |
| `RequestIDFromContext(ctx)` | Retrieves the ID (returns `""` if absent) |

The `RequestIDMiddleware` calls these automatically. Handlers access the ID via `observability.RequestIDFromContext(ctx)`. It is also set as the `X-Request-ID` response header and included in all JSON error responses.

---

## Middleware Stack

**File:** `internal/observability/middleware.go`

Three middlewares are applied to every route in `internal/server/router.go`, in this order:

```go
r.Use(observability.RequestIDMiddleware)   // 1st — outermost
r.Use(observability.TracingMiddleware)     // 2nd
r.Use(observability.LoggingMiddleware)     // 3rd — innermost
```

### Execution flow for an incoming request:

```
Request arrives
  -> RequestIDMiddleware: generate UUID, store in context, set X-Request-ID header
    -> TracingMiddleware (otelhttp): create root span, inject SpanContext into context
      -> LoggingMiddleware: capture start time, get trace-correlated logger
        -> Handler executes (may create child spans, record metrics, log)
      <- LoggingMiddleware: log "request completed" with method, path, request_id, duration
    <- TracingMiddleware: end root span, record HTTP status/duration
  <- RequestIDMiddleware: (no post-processing)
Response sent
```

**The order matters:**
- `RequestIDMiddleware` must run first so the ID is available to all downstream middleware and handlers.
- `TracingMiddleware` must run before `LoggingMiddleware` so the `SpanContext` is in the Go context when `LoggerWithTrace` reads it.
- `LoggingMiddleware` runs innermost so it can measure the actual handler duration and log after completion.

Handlers remain clean — all cross-cutting observability concerns are handled by middleware.

---

## Shared Error Handling

**File:** `internal/observability/errors.go`

`RecordError` is a single function that performs all four error actions in one call:

```go
func RecordError(
    ctx     context.Context,
    span    trace.Span,
    logger  *zap.Logger,
    counter metric.Int64Counter,  // domain's error counter — passed in
    opName  string,
    msg     string,
    err     error,
    status  int,
    w       http.ResponseWriter,
)
```

What it does:
1. **Span:** calls `span.RecordError(err)` and `span.SetStatus(codes.Error, msg)`
2. **Metric:** increments the provided counter with an `operation` attribute
3. **Log:** emits an error-level log with operation name, error, and request ID
4. **Response:** writes a JSON error response `{"error": "...", "request_id": "..."}`

The error counter is passed as a parameter (not hardcoded) so this function is domain-agnostic. Each domain passes its own counter.

**For complex error scenarios** (e.g. errors that need to be recorded on multiple spans), use the individual OTel/Zap calls directly and `handlers.WriteError()` for the HTTP response. See the chain handler in `internal/calculator/handlers.go` for an example.

**File:** `internal/handlers/response.go`

`handlers.WriteError(w, status, msg, requestID)` writes a standardised JSON error response. Use this when you need to write an error response without the full span+metric+log ceremony (e.g. when you've already handled those separately).

---

## Instrumenting New Functionality

### Step-by-step Checklist

When adding a new handler or feature, follow this checklist to ensure full observability coverage:

- [ ] Get a trace-correlated logger: `logger := observability.LoggerWithTrace(ctx)`
- [ ] Get the request ID: `requestID := observability.RequestIDFromContext(ctx)`
- [ ] Create a custom child span with meaningful name and attributes
- [ ] Record domain-specific metrics (counter, histogram, gauge)
- [ ] Use `observability.RecordError()` for error paths
- [ ] Log key business events with structured fields
- [ ] Include `request_id` in all JSON responses

### Creating Custom Spans

Each domain package should declare its own tracer at package level:

```go
var tracer = otel.Tracer("mydomainname")
```

Then create child spans inside handlers:

```go
func MyHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    ctx, span := tracer.Start(ctx, "mydomain.operation_name",
        trace.WithAttributes(
            attribute.String("mydomain.some_key", someValue),
            attribute.String("request.id", observability.RequestIDFromContext(ctx)),
        ),
    )
    defer span.End()

    // ... handler logic ...

    // Add events for significant moments:
    span.AddEvent("step.completed", trace.WithAttributes(
        attribute.String("detail", "value"),
    ))

    // Set result attributes:
    span.SetAttributes(attribute.Int("mydomain.result_count", count))

    // Mark success:
    span.SetStatus(codes.Ok, "")
}
```

**Naming conventions:**
- Tracer name: the domain name (e.g. `"calculator"`, `"users"`, `"orders"`)
- Span names: `domain.operation` (e.g. `"calculator.add"`, `"users.create"`)
- Attribute keys: `domain.noun` (e.g. `"calculator.operand.a"`, `"users.email"`)
- Event names: `noun.verb` (e.g. `"computation.complete"`, `"validation.failed"`)

### Defining Domain Metrics

Each domain defines its metric instruments in `metrics.go`:

```go
package mydomain

import (
    "fmt"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/metric"
)

var (
    opsCounter   metric.Int64Counter
    opsHistogram metric.Float64Histogram
    errorCounter metric.Int64Counter
)

func InitMetrics() error {
    meter := otel.Meter("mydomain")

    var err error

    opsCounter, err = meter.Int64Counter("mydomain.operations.total",
        metric.WithDescription("Total number of mydomain operations"),
        metric.WithUnit("{operation}"),
    )
    if err != nil {
        return fmt.Errorf("creating ops counter: %w", err)
    }

    // ... more instruments ...

    return nil
}
```

**Naming conventions:**
- Meter name: same as the tracer name (the domain name)
- Metric names: `domain.noun.suffix` (e.g. `"calculator.operations.total"`)
- Suffixes: `.total` for counters, `.duration` for histograms, no suffix or descriptive for gauges
- Units: `"ms"` for durations, `"{operation}"` for counted things, `"1"` for dimensionless values

**Common metric patterns:**

| Instrument | Use case | Example |
|---|---|---|
| `Int64Counter` | Count events | `mydomain.operations.total`, `mydomain.errors.total` |
| `Float64Histogram` | Measure durations/distributions | `mydomain.operation.duration` (ms) |
| `Float64Gauge` | Track current values | `mydomain.last_result`, `mydomain.queue_size` |

**Recording metrics in handlers:**

```go
attrs := metric.WithAttributes(attribute.String("operation", opName))
opsCounter.Add(ctx, 1, attrs)
opsHistogram.Record(ctx, elapsedMs, attrs)
```

Always pass `ctx` — the OTel SDK uses it for context propagation. Always include meaningful attributes to enable filtering/grouping in dashboards.

**Registration:** After creating `InitMetrics()`, add it to `cmd/api/init.go`:

```go
if err := mydomain.InitMetrics(); err != nil {
    return nil, err
}
```

### Logging Correctly

Always use the trace-correlated logger:

```go
logger := observability.LoggerWithTrace(ctx)
```

**Do:**
- Include `request_id` in business-relevant log lines
- Use structured fields (`zap.String`, `zap.Int`, `zap.Float64`, `zap.Error`)
- Log at `Info` for successful operations, `Error` for failures
- Include operation name, inputs, outputs, and duration

```go
logger.Info("order created",
    zap.String("operation", "create"),
    zap.String("order_id", orderID),
    zap.String("request_id", requestID),
    zap.Float64("total", total),
    zap.Float64("duration_ms", elapsed),
)
```

**Don't:**
- Use `fmt.Println` or the standard `log` package
- Use `observability.Logger` directly (loses trace correlation)
- Log sensitive data (passwords, tokens, PII)
- Over-log inside tight loops (use span events instead)

### Handling Errors

**Simple case — one span, standard error response:**

```go
if err != nil {
    observability.RecordError(ctx, span, logger, errorCounter, opName, "descriptive message", err, http.StatusBadRequest, w)
    return
}
```

This records the error on the span, increments your domain's error counter, logs at error level with trace correlation, and writes the JSON error response. One line covers all four concerns.

**Complex case — multiple spans or custom error handling:**

When you need to record errors on multiple spans (e.g. a child span and a parent span), handle each span separately and use `handlers.WriteError()` for the response:

```go
if err != nil {
    // Child span
    childSpan.RecordError(err)
    childSpan.SetStatus(codes.Error, err.Error())
    childSpan.End()

    // Parent span
    parentSpan.RecordError(err)
    parentSpan.SetStatus(codes.Error, "child failed")

    // Metric + log
    errorCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", opName)))
    logger.Error("child operation failed", zap.Error(err), zap.String("request_id", requestID))

    // HTTP response
    handlers.WriteError(w, http.StatusBadRequest, err.Error(), requestID)
    return
}
```

### Nested Spans

For operations with multiple steps (batch processing, pipelines, chained operations), create child spans within a parent span to produce a multi-level trace tree:

```go
// Parent span
ctx, parentSpan := tracer.Start(ctx, "mydomain.pipeline")
defer parentSpan.End()

for i, step := range steps {
    // Child span — uses ctx from parent, so it becomes a child
    _, stepSpan := tracer.Start(ctx, fmt.Sprintf("mydomain.pipeline.step.%d", i))

    // ... do work ...

    stepSpan.SetStatus(codes.Ok, "")
    stepSpan.End()  // Must End() explicitly in loops (defer won't work)
}

parentSpan.SetStatus(codes.Ok, "")
```

This produces a trace tree like:

```
mydomain.pipeline (parent)
  ├── mydomain.pipeline.step.0
  ├── mydomain.pipeline.step.1
  └── mydomain.pipeline.step.2
```

Visible as a waterfall in Jaeger, Grafana Tempo, or any OTel-compatible trace viewer.

---

## Environment Variables

All OTel configuration is driven by standard environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `OTEL_SERVICE_NAME` | `go-chi-api` | Service name in traces and metrics |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://localhost:4318` | OTLP collector endpoint (HTTP) |
| `OTEL_EXPORTER_OTLP_HEADERS` | (none) | Auth headers for the OTLP exporter |
| `OTEL_RESOURCE_ATTRIBUTES` | (none) | Additional resource attributes (e.g. `deployment.environment=prod`) |

No application code changes are needed to switch between local development (Jaeger) and production (Grafana Cloud, Datadog, etc.) — just set the environment variables.
