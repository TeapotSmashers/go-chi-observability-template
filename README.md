# go-chi-observability-template

A production-ready Go HTTP API boilerplate with observability as a first-class concern. Built on [go-chi/chi](https://github.com/go-chi/chi) with OpenTelemetry tracing, OpenTelemetry metrics, Prometheus metrics, and Zap structured logging with OTel log export — all wired together with automatic trace correlation and cross-linked in Grafana.

Start building your API with full observability from day one instead of bolting it on later.

## What's Included

| Pillar | Implementation | Transport |
|--------|---------------|-----------|
| **Distributed Tracing** | OpenTelemetry SDK | OTLP/HTTP push to any collector |
| **Metrics (push)** | OpenTelemetry SDK | OTLP/HTTP push to any collector |
| **Metrics (pull)** | Prometheus client | `GET /metrics` scrape endpoint |
| **Structured Logging** | Zap (JSON) | stdout + OTLP/HTTP push via OTel Zap bridge |
| **Log-Trace Correlation** | Automatic | `trace_id` + `span_id` on every log line |
| **Request IDs** | UUID v4 | Context propagation + `X-Request-ID` header |

All three observability pillars are connected: logs carry trace IDs and are exported to Loki via OTLP, spans carry request IDs, and errors are recorded across all systems in a single function call. In Grafana, Loki logs link to Tempo traces and vice versa.

## Quick Start

```bash
# Clone and build
git clone <repo-url> && cd go-chi-observability-template
go build ./...

# Run (no external dependencies needed — OTel exporters fail silently if no collector is running)
go run ./cmd/api

# Test it
curl http://localhost:8080/health

curl -X POST http://localhost:8080/calculator/add \
  -H 'Content-Type: application/json' \
  -d '{"a": 10, "b": 5}'
```

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Liveness check |
| `GET` | `/metrics` | Prometheus scrape endpoint |
| `POST` | `/calculator/add` | Add two numbers |
| `POST` | `/calculator/subtract` | Subtract two numbers |
| `POST` | `/calculator/multiply` | Multiply two numbers |
| `POST` | `/calculator/divide` | Divide (demonstrates error path observability) |
| `POST` | `/calculator/chain` | Chained operations (demonstrates nested spans) |

The calculator domain is a **reference implementation** — it exists to demonstrate every observability pattern. Use it as a template when building real domains.

### Example Requests

```bash
# Basic arithmetic
curl -X POST http://localhost:8080/calculator/multiply \
  -H 'Content-Type: application/json' \
  -d '{"a": 7, "b": 6}'

# Error path — division by zero
curl -X POST http://localhost:8080/calculator/divide \
  -H 'Content-Type: application/json' \
  -d '{"a": 10, "b": 0}'

# Chained calculation: (10 + 5) * 3 / 2 = 22.5
# Produces nested spans — one per step
curl -X POST http://localhost:8080/calculator/chain \
  -H 'Content-Type: application/json' \
  -d '{
    "initial": 10,
    "steps": [
      {"op": "add", "value": 5},
      {"op": "multiply", "value": 3},
      {"op": "divide", "value": 2}
    ]
  }'
```

Every response includes a `request_id` for correlation:

```json
{
  "operation": "add",
  "a": 10,
  "b": 5,
  "result": 15,
  "request_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479"
}
```

## How Observability Works

### Automatic (zero handler code)

The middleware stack handles these for every request without any code in handlers:

- **Request ID** — UUID v4 generated, stored in context, set as `X-Request-ID` response header
- **Distributed trace** — root span created via `otelhttp`, W3C `traceparent` propagated
- **Structured request log** — method, path, request ID, duration, trace ID, span ID

### Per-handler (explicit instrumentation)

Handlers add domain-specific observability on top:

```go
func MyHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    logger := observability.LoggerWithTrace(ctx)       // logger with trace_id + span_id
    requestID := observability.RequestIDFromContext(ctx) // request UUID

    ctx, span := tracer.Start(ctx, "mydomain.operation") // custom child span
    defer span.End()

    // ... business logic ...

    opsCounter.Add(ctx, 1, attrs)                       // custom metric
    logger.Info("operation completed", zap.String("request_id", requestID))
}
```

### Error handling

One function records an error across all four systems:

```go
observability.RecordError(ctx, span, logger, errorCounter, opName, msg, err, status, w)
// Records on span, increments metric, logs with trace context, writes JSON error response
```

## Project Structure

```
cmd/api/
  main.go              # Entrypoint — init order, server start, graceful shutdown
  init.go              # Composition root — wires domain metric initialisers

internal/
  observability/        # Generic infrastructure (never imports domain packages)
    errors.go           # RecordError() — shared error handling
    logger.go           # Zap logger + trace correlation
    logging.go          # OTel LoggerProvider + Zap bridge (logs → OTLP)
    metrics.go          # OTel MeterProvider + Prometheus
    middleware.go       # RequestID, Tracing, Logging middlewares
    request_id.go       # UUID request ID + context helpers
    tracing.go          # OTel TracerProvider

  handlers/             # Shared handler utilities
    health.go           # GET /health
    response.go         # WriteError() — JSON error responses

  calculator/           # Example domain (reference implementation)
    types.go            # Request/response structs
    metrics.go          # OTel metric instruments + InitMetrics()
    handlers.go         # HTTP handlers + tracer
    routes.go           # RegisterRoutes(r chi.Router)

  server/
    router.go           # Chi router — middleware + route composition

docs/
  observability.md      # Observability internals + instrumentation guide
  api-structure.md      # Package conventions + full walkthrough
```

## Adding a New Domain

Every API domain follows the same four-file pattern. Adding one requires **two one-line changes** to existing files:

### 1. Create the domain package

```
internal/newdomain/
  types.go       # Request/response structs
  metrics.go     # Metric instruments + InitMetrics()
  handlers.go    # Handlers + tracer
  routes.go      # RegisterRoutes(r chi.Router)
```

### 2. Wire it up

**`internal/server/router.go`** — add one line:
```go
newdomain.RegisterRoutes(r)
```

**`cmd/api/init.go`** — add one line:
```go
if err := newdomain.InitMetrics(); err != nil {
    return nil, err
}
```

`main.go` never changes. See [docs/api-structure.md](docs/api-structure.md) for the complete walkthrough with code examples.

## Configuration

All OpenTelemetry configuration is driven by standard environment variables — no code changes needed to switch between local dev and production:

The API automatically loads variables from `.env` on startup (when the file exists), while still letting real environment variables take precedence.

| Variable | Default | Purpose |
|---|---|---|
| `OTEL_SERVICE_NAME` | `go-chi-api` | Service name in traces, metrics, and logs |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://localhost:4318` | OTLP collector endpoint |
| `OTEL_EXPORTER_OTLP_HEADERS` | — | Auth headers (e.g. for Grafana Cloud) |
| `OTEL_RESOURCE_ATTRIBUTES` | — | Extra attributes (e.g. `deployment.environment=prod`) |

### Local development with Jaeger

```bash
# Start Jaeger with OTLP support
docker run -d --name jaeger \
  -p 4318:4318 \
  -p 16686:16686 \
  jaegertracing/all-in-one:latest

# Run the API
go run ./cmd/api

# Send some requests, then view traces at http://localhost:16686
```

### Production with Grafana Cloud

```bash
export OTEL_SERVICE_NAME=my-api
export OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp-gateway-prod-us-east-0.grafana.net/otlp
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Basic <base64-encoded>"
export OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production"
go run ./cmd/api
```

## Tech Stack

| Dependency | Version | Purpose |
|---|---|---|
| [go-chi/chi](https://github.com/go-chi/chi) | v5.2.5 | HTTP router |
| [uber/zap](https://github.com/uber-go/zap) | v1.27.1 | Structured logging |
| [OpenTelemetry](https://opentelemetry.io/) | v1.40.0 | Tracing + metrics + logs SDK |
| [otelzap](https://pkg.go.dev/go.opentelemetry.io/contrib/bridges/otelzap) | — | Zap → OTel log bridge |
| [otelhttp](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp) | v0.65.0 | Automatic HTTP instrumentation |
| [prometheus/client_golang](https://github.com/prometheus/client_golang) | v1.23.2 | Prometheus `/metrics` endpoint |
| [google/uuid](https://github.com/google/uuid) | v1.6.0 | Request ID generation |

## Documentation

- **[docs/observability.md](docs/observability.md)** — Deep-dive into the observability infrastructure internals and a complete guide on how to instrument new functionality
- **[docs/api-structure.md](docs/api-structure.md)** — Package structure conventions, naming rules, dependency directions, and a step-by-step walkthrough for adding new domains
- **[AGENTS.md](AGENTS.md)** — Context for AI coding agents working on this codebase

## License

MIT
