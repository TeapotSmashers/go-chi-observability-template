# AGENTS.md

This file provides context for AI coding agents working on this codebase.

## Project Overview

This is a Go HTTP API boilerplate built on [go-chi/chi](https://github.com/go-chi/chi) with production-grade observability baked in: OpenTelemetry tracing, OpenTelemetry metrics (OTLP push), Prometheus metrics (pull), and Zap structured logging with automatic trace correlation.

The project serves as a template — the `calculator` domain is included as a reference implementation demonstrating all observability patterns.

## Tech Stack

- **Language:** Go 1.25+
- **Router:** go-chi/chi v5
- **Logging:** go.uber.org/zap (production JSON)
- **Tracing:** OpenTelemetry SDK with OTLP/HTTP exporter
- **Metrics:** OpenTelemetry SDK (OTLP/HTTP push) + Prometheus (HTTP pull at `/metrics`)
- **Request IDs:** google/uuid v4

## Project Structure

```
cmd/api/
  main.go           # Entrypoint — init order, server start, graceful shutdown
  init.go           # Composition root — wires domain metric initialisers

internal/
  observability/     # Generic infra — NEVER import domain packages from here
    errors.go        # RecordError() — shared span+metric+log+response helper
    logger.go        # Zap logger + LoggerWithTrace(ctx) for trace correlation
    metrics.go       # OTel MeterProvider + Prometheus handler
    middleware.go    # RequestID, Tracing, Logging middlewares (applied to all routes)
    request_id.go    # UUID request ID + context helpers
    tracing.go       # OTel TracerProvider (OTLP/HTTP)

  handlers/          # Shared handler utilities
    health.go        # GET /health
    response.go      # WriteError() — shared JSON error response writer

  calculator/        # Example domain package (reference implementation)
    types.go         # Request/response structs
    metrics.go       # OTel metric instruments + InitMetrics()
    handlers.go      # HTTP handlers + tracer + domain helpers
    routes.go        # RegisterRoutes(r chi.Router)

  server/
    router.go        # Chi router — middleware stack + route composition

docs/
  observability.md   # Deep-dive on observability internals + instrumentation guide
  api-structure.md   # Package structure, naming, and conventions
  otel-collect.md    # Local OTLP backend setup + Grafana visualization guide

otel-collect/          # Docker-based observability backend
  docker-compose.yml           # All services: OTel Collector, Tempo, Prometheus, Loki, Grafana
  otel-collector-config.yaml   # Collector pipeline: OTLP → Tempo (traces) + Prometheus (metrics)
  tempo.yaml                   # Grafana Tempo trace storage config
  prometheus.yaml              # Prometheus scrape targets
  grafana-datasources.yaml     # Auto-provisioned Grafana datasources
```

## Key Architecture Rules

1. **Domain packages are self-contained.** Each domain (e.g. `internal/calculator/`) owns its types, metrics, handlers, and routes in four files: `types.go`, `metrics.go`, `handlers.go`, `routes.go`.

2. **Dependency direction is strictly one-way:**
   ```
   cmd/api/ -> internal/server, internal/observability, internal/<domain>
   internal/server/ -> internal/observability, internal/handlers, internal/<domain>
   internal/<domain>/ -> internal/observability, internal/handlers
   internal/handlers/ -> standard library only
   internal/observability/ -> external libs only (no internal imports)
   ```

3. **Domain packages never import other domain packages.** If cross-domain logic is needed, extract it into a shared package.

4. **`internal/observability/` is domain-agnostic.** It must never import any domain package. The `RecordError` function takes the error counter as a parameter to stay generic.

5. **`main.go` never changes** when adding new domains. Only `init.go` (for metrics) and `router.go` (for routes) get one new line each.

## Adding a New Domain

When asked to create new API functionality, follow these steps:

1. Create `internal/<domain>/` with four files:
   - `types.go` — request/response structs (JSON tags, no logic)
   - `metrics.go` — OTel metric instruments + `InitMetrics()` function
   - `handlers.go` — HTTP handlers + package-level tracer (`otel.Tracer("domain")`)
   - `routes.go` — `RegisterRoutes(r chi.Router)` mounting routes under `/<domain>`

2. Wire it up (two one-line additions):
   - `internal/server/router.go`: add `domain.RegisterRoutes(r)`
   - `cmd/api/init.go`: add `domain.InitMetrics()` call

3. See `docs/api-structure.md` for the complete walkthrough with code examples.

## Observability Instrumentation Checklist

Every handler should:

- [ ] Get a trace-correlated logger: `logger := observability.LoggerWithTrace(ctx)`
- [ ] Get the request ID: `requestID := observability.RequestIDFromContext(ctx)`
- [ ] Create a custom child span: `ctx, span := tracer.Start(ctx, "domain.operation")`
- [ ] Record domain-specific metrics (counters, histograms, gauges)
- [ ] Use `observability.RecordError()` for error paths (handles span + metric + log + response)
- [ ] Log key business events with structured fields including `request_id`
- [ ] Include `request_id` in all JSON responses

See `docs/observability.md` for the full instrumentation guide.

## Naming Conventions

- **Package names:** lowercase single noun — `calculator`, `users`, `orders`
- **Tracer/Meter names:** match the package name — `otel.Tracer("calculator")`
- **Span names:** `domain.action` — `"calculator.add"`, `"users.create"`
- **Metric names:** `domain.noun.suffix` — `"calculator.operations.total"`
- **Attribute keys:** `domain.noun` — `"calculator.operand.a"`
- **Route prefixes:** `/<domain>` — `/calculator`, `/users`

## Building and Running

```bash
go build ./...                    # Build
go run ./cmd/api                  # Run (listens on :8080)
```

Environment variables for OTel configuration:
- `OTEL_SERVICE_NAME` (default: `go-chi-api`)
- `OTEL_EXPORTER_OTLP_ENDPOINT` (default: `http://localhost:4318`)
- `OTEL_RESOURCE_ATTRIBUTES` (e.g. `deployment.environment=staging`)

## Local Observability Backend

A Docker-based OTLP backend lives in `otel-collect/`. It provides an OTel Collector, Grafana Tempo (traces), Prometheus (metrics), Loki (logs), and Grafana (visualization).

```bash
# Start the backend
docker-compose -f otel-collect/docker-compose.yml up -d

# Run the API connected to the backend
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 go run ./cmd/api

# Stop the backend
docker-compose -f otel-collect/docker-compose.yml down

# Stop and wipe all data
docker-compose -f otel-collect/docker-compose.yml down -v
```

| Service | URL | Purpose |
|---------|-----|---------|
| Grafana | http://localhost:3000 | Dashboards, trace explorer, metric queries |
| Prometheus | http://localhost:9090 | Raw PromQL queries, target health |
| Tempo | http://localhost:3200 | Trace API (usually accessed via Grafana) |

See `docs/otel-collect.md` for the full configuration guide.

## Existing Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness check — returns `200 ok` |
| GET | `/metrics` | Prometheus scrape endpoint |
| POST | `/calculator/add` | Add two numbers |
| POST | `/calculator/subtract` | Subtract two numbers |
| POST | `/calculator/multiply` | Multiply two numbers |
| POST | `/calculator/divide` | Divide (with division-by-zero error path) |
| POST | `/calculator/chain` | Chained operations with nested spans |

## Important Files to Read First

When orienting in this codebase, read these in order:
1. `cmd/api/main.go` — understand the init and startup sequence
2. `internal/observability/middleware.go` — understand the middleware stack
3. `internal/calculator/handlers.go` — reference implementation for new domains
4. `docs/observability.md` — complete instrumentation guide
5. `docs/api-structure.md` — package structure conventions
6. `docs/otel-collect.md` — local OTLP backend setup and Grafana visualization
