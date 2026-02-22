# Local Observability Backend (`otel-collect/`)

This document describes the Docker-based OTLP backend that collects traces, metrics, and logs from the Go API and provides a Grafana UI for visualization.

## Table of Contents

- [Architecture](#architecture)
- [Data Flow](#data-flow)
- [Services](#services)
  - [OpenTelemetry Collector](#opentelemetry-collector)
  - [Grafana Tempo](#grafana-tempo)
  - [Prometheus](#prometheus)
  - [Loki](#loki)
  - [Grafana](#grafana)
- [Quick Start](#quick-start)
- [Connecting the Go API](#connecting-the-go-api)
- [Accessing the UIs](#accessing-the-uis)
- [Grafana MCP Server](#grafana-mcp-server)
- [Configuration Files](#configuration-files)
  - [`docker-compose.yml`](#docker-composeyml)
  - [`otel-collector-config.yaml`](#otel-collector-configyaml)
  - [`tempo.yaml`](#tempoyaml)
  - [`prometheus.yaml`](#prometheusyaml)
  - [`grafana-datasources.yaml`](#grafana-datasourcesyaml)
- [Common Tasks](#common-tasks)
  - [Viewing Traces](#viewing-traces)
  - [Querying Metrics](#querying-metrics)
  - [Resetting Data](#resetting-data)
- [Troubleshooting](#troubleshooting)

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Host machine                                                   │
│                                                                 │
│  ┌──────────────┐                                               │
│  │  Go API      │                                               │
│  │  :8080       │                                               │
│  └──────┬───────┘                                               │
│         │  OTLP/HTTP (:4318)          Prometheus scrape (:8080) │
│         ▼                                    ▲                  │
│  ┌──────────────────┐                        │                  │
│  │  OTel Collector   │                        │                  │
│  │  :4317 :4318      │                        │                  │
│  │  :8888 :8889      │                        │                  │
│  └──┬──────────┬─────┘                        │                  │
│     │ traces   │ metrics                      │                  │
│     ▼          ▼                              │                  │
│  ┌────────┐ ┌──────────┐                      │                  │
│  │ Tempo  │ │Prometheus│──────────────────────┘                  │
│  │ :3200  │ │  :9090   │                                        │
│  └────┬───┘ └────┬─────┘                                        │
│       │          │                                               │
│       ▼          ▼                                               │
│  ┌─────────────────┐                                            │
│  │    Grafana       │                                            │
│  │    :3000         │                                            │
│  └─────────────────┘                                            │
└─────────────────────────────────────────────────────────────────┘
```

## Data Flow

There are two independent data paths:

1. **Traces:** Go API pushes OTLP/HTTP to the OTel Collector on port `4318`. The Collector batches and forwards traces via OTLP to Grafana Tempo. Grafana queries Tempo to display trace waterfalls.

2. **Metrics (push):** Go API pushes OTLP/HTTP metrics to the OTel Collector on port `4318`. The Collector converts them to Prometheus format and exposes them on port `8889`. Prometheus scrapes port `8889` to ingest the pushed metrics.

3. **Metrics (pull):** Prometheus also directly scrapes the Go API's `/metrics` endpoint on port `8080` for Prometheus-native metrics (Go runtime stats, etc.).

Grafana is pre-configured with Tempo and Prometheus as datasources and provides the visualization layer for both.

---

## Services

### OpenTelemetry Collector

- **Image:** `otel/opentelemetry-collector-contrib:latest`
- **Role:** Central telemetry gateway — receives all OTLP data from the Go API and routes it to the appropriate backends.
- **Ports:**

| Port | Protocol | Purpose |
|------|----------|---------|
| 4317 | gRPC | OTLP gRPC receiver |
| 4318 | HTTP | OTLP HTTP receiver (used by the Go API) |
| 8888 | HTTP | Collector's own internal metrics |
| 8889 | HTTP | Prometheus exporter — exposes app metrics for Prometheus to scrape |

- **Config:** `otel-collector-config.yaml`

### Grafana Tempo

- **Image:** `grafana/tempo:latest`
- **Role:** Distributed tracing backend — stores and indexes trace spans.
- **Ports:**

| Port | Protocol | Purpose |
|------|----------|---------|
| 3200 | HTTP | Tempo API (queried by Grafana) |
| 4316 | gRPC | OTLP gRPC receiver (internal) |
| 4311 | HTTP | OTLP HTTP receiver (internal) |

- **Storage:** Local filesystem at `/var/tempo` (Docker volume `tempo-data`).
- **Config:** `tempo.yaml`

### Prometheus

- **Image:** `prom/prometheus:latest`
- **Role:** Metrics storage — scrapes both the OTel Collector's Prometheus exporter and the Go API's `/metrics` endpoint.
- **Port:** `9090` (Prometheus UI and API)
- **Scrape targets:**
  - `otel-collector:8888` — Collector internal metrics
  - `otel-collector:8889` — Application metrics forwarded through the Collector
  - `host.docker.internal:8080/metrics` — Go API Prometheus endpoint (Go runtime metrics)
- **Config:** `prometheus.yaml`

### Loki

- **Image:** `grafana/loki:latest`
- **Role:** Log aggregation backend. Currently available for future use — the Go API does not push logs to Loki by default. Logs are written to stdout as structured JSON and can be collected by a log shipper if needed.
- **Port:** `3100`
- **Storage:** Docker volume `loki-data`

### Grafana

- **Image:** `grafana/grafana:latest`
- **Role:** Visualization and exploration UI. Pre-configured with Tempo and Prometheus as datasources.
- **Port:** `3000`
- **Auth:** Anonymous access enabled with Admin role — no login required.
- **Feature flags:** `traceqlEditor` enabled for TraceQL query editor.
- **Pre-provisioned datasources:**
  - **Tempo** — `http://tempo:3200`
  - **Prometheus** (default) — `http://prometheus:9090`
- **Config:** `grafana-datasources.yaml`

---

## Quick Start

### Start the backend

```bash
docker-compose -f otel-collect/docker-compose.yml up -d
```

### Verify all services are running

```bash
docker ps --filter "name=otel-collect" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
```

All five containers should show `Up`:

```
NAMES                           STATUS       PORTS
otel-collect-grafana-1          Up ...       0.0.0.0:3000->3000/tcp
otel-collect-otel-collector-1   Up ...       0.0.0.0:4317-4318->4317-4318/tcp, ...
otel-collect-tempo-1            Up ...       0.0.0.0:3200->3200/tcp, ...
otel-collect-prometheus-1       Up ...       0.0.0.0:9090->9090/tcp
otel-collect-loki-1             Up ...       0.0.0.0:3100->3100/tcp
```

### Stop the backend

```bash
docker-compose -f otel-collect/docker-compose.yml down
```

### Stop and remove all data

```bash
docker-compose -f otel-collect/docker-compose.yml down -v
```

---

## Connecting the Go API

The Go API uses the standard `OTEL_EXPORTER_OTLP_ENDPOINT` environment variable (see `internal/observability/tracing.go` and `internal/observability/metrics.go`). Point it at the OTel Collector:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 go run ./cmd/api
```

That's it. Both traces (via `otlptracehttp`) and metrics (via `otlpmetrichttp`) are sent to the Collector, which routes them to Tempo and Prometheus respectively.

You can also set `OTEL_SERVICE_NAME` to customize how the service appears in Grafana:

```bash
OTEL_SERVICE_NAME=my-api \
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 \
go run ./cmd/api
```

### Generate sample data

Once the API is running, send some requests to generate traces and metrics:

```bash
# Simple operation
curl -X POST http://localhost:8080/calculator/add \
  -H 'Content-Type: application/json' \
  -d '{"a": 10, "b": 20}'

# Error path (division by zero)
curl -X POST http://localhost:8080/calculator/divide \
  -H 'Content-Type: application/json' \
  -d '{"a": 10, "b": 0}'

# Chained operations (produces nested spans)
curl -X POST http://localhost:8080/calculator/chain \
  -H 'Content-Type: application/json' \
  -d '{"initial_value": 10, "steps": [{"operation": "add", "value": 5}, {"operation": "multiply", "value": 3}]}'
```

---

## Accessing the UIs

| Service | URL | Purpose |
|---------|-----|---------|
| Grafana | http://localhost:3000 | Dashboards, trace explorer, metric queries |
| Prometheus | http://localhost:9090 | Raw PromQL queries, target health |
| Tempo | http://localhost:3200 | Tempo API (usually accessed via Grafana) |

---

## Grafana MCP Server

An MCP (Model Context Protocol) server for Grafana is configured in `~/.config/opencode/opencode.json` under the `grafana` key. This allows AI agents (like OpenCode) to interact with Grafana programmatically — querying datasources, searching dashboards, and running PromQL/TraceQL queries.

The MCP server runs as a Docker container using stdio transport and connects to the local Grafana instance:

```json
{
  "grafana": {
    "type": "local",
    "enabled": true,
    "command": [
      "docker", "run", "--rm", "-i",
      "-e", "GRAFANA_URL",
      "grafana/mcp-grafana",
      "-t", "stdio"
    ],
    "environment": {
      "GRAFANA_URL": "http://host.docker.internal:3000"
    }
  }
}
```

No API key is needed because Grafana is configured with anonymous admin access. The `host.docker.internal` hostname allows the MCP container to reach the Grafana container exposed on the host's port 3000.

---

## Configuration Files

All configuration files live in the `otel-collect/` directory.

### `docker-compose.yml`

Defines all five services (otel-collector, tempo, prometheus, loki, grafana) and their Docker volumes. Key configuration choices:

- **Grafana anonymous auth:** `GF_AUTH_ANONYMOUS_ENABLED=true` with `GF_AUTH_ANONYMOUS_ORG_ROLE=Admin` — no login barrier for local development.
- **Datasource provisioning:** `grafana-datasources.yaml` is mounted into Grafana's provisioning directory so Tempo and Prometheus are available immediately.
- **Service dependencies:** The OTel Collector depends on Tempo and Prometheus. Grafana depends on Prometheus and Tempo.

### `otel-collector-config.yaml`

Defines the Collector's pipeline:

```
Receivers          Processors        Exporters
─────────          ──────────        ─────────
otlp (HTTP+gRPC)   batch             traces  → otlp (Tempo)
                                     metrics → prometheus (port 8889)
```

- **Batch processor:** Buffers spans/metrics for 10 seconds or 1000 items before flushing — reduces network overhead.
- **Prometheus exporter:** Converts OTLP metrics to Prometheus format under the `otel` namespace with a `service=go-chi-api` label.
- **OTLP exporter:** Forwards traces to Tempo on port `4311` (HTTP) with TLS disabled (internal Docker network).

### `tempo.yaml`

Minimal Tempo configuration:

- Listens on port `3200` (HTTP API) and `4316` (gRPC)
- Accepts OTLP traces via the distributor
- Stores traces locally at `/var/tempo` with a 5-minute block duration
- Uses WAL (write-ahead log) for durability

### `prometheus.yaml`

Defines two scrape jobs:

| Job | Target | What it collects |
|-----|--------|-----------------|
| `opentelemetry-collector` | `otel-collector:8888`, `otel-collector:8889` | Collector internal metrics + app metrics forwarded through the Collector |
| `go-chi-api` | `host.docker.internal:8080/metrics` | Go runtime metrics from the API's Prometheus endpoint |

Scrape interval is 15 seconds.

### `grafana-datasources.yaml`

Auto-provisions two datasources in Grafana on startup:

- **Prometheus** (default) — for metric queries via PromQL
- **Tempo** — for trace queries via TraceQL

Both are marked as editable so you can modify them in the Grafana UI if needed.

---

## Common Tasks

### Viewing Traces

1. Open Grafana at http://localhost:3000
2. Click **Explore** in the left sidebar
3. Select **Tempo** from the datasource dropdown
4. Use the **Search** tab to find traces by service name, span name, duration, or status
5. Click a trace ID to view the full waterfall diagram with span details

Example TraceQL query to find slow calculator operations:

```
{ resource.service.name = "go-chi-api" && span.http.route = "/calculator/*" } | duration > 100ms
```

### Querying Metrics

1. Open Grafana at http://localhost:3000
2. Click **Explore** in the left sidebar
3. Select **Prometheus** from the datasource dropdown
4. Enter a PromQL query

Example queries:

```promql
# Total calculator operations by type
otel_calculator_operations_total

# Error rate over the last 5 minutes
rate(otel_calculator_errors_total[5m])

# Go runtime — number of goroutines
go_goroutines
```

Metrics pushed through the OTel Collector are prefixed with `otel_` (configured via the `namespace` setting in the Collector's Prometheus exporter).

### Resetting Data

To clear all stored traces, metrics, and dashboards:

```bash
docker-compose -f otel-collect/docker-compose.yml down -v
docker-compose -f otel-collect/docker-compose.yml up -d
```

The `-v` flag removes all Docker volumes, giving you a clean slate.

---

## Troubleshooting

### A container keeps exiting

Check its logs:

```bash
docker logs otel-collect-tempo-1
docker logs otel-collect-otel-collector-1
```

Common causes:
- **Tempo:** Invalid YAML in `tempo.yaml` — the `distributor.receivers` section must use map syntax (`otlp:`) not list syntax (`- otlp`).
- **OTel Collector:** Invalid exporter name or unreachable backend — check that Tempo and Prometheus are healthy before the Collector starts.

### No traces appearing in Grafana

1. Verify the API is sending to the right endpoint: `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`
2. Check the Collector logs for errors: `docker logs otel-collect-otel-collector-1`
3. Verify Tempo is running: `docker ps --filter "name=tempo"`
4. In Grafana Explore, make sure the **Tempo** datasource is selected

### No metrics appearing in Prometheus

1. Check Prometheus target health at http://localhost:9090/targets — all targets should be `UP`
2. If `go-chi-api` target is `DOWN`, ensure the API is running on port 8080
3. If `opentelemetry-collector` targets are `DOWN`, check that the Collector is running

### Port conflicts

If a port is already in use, modify the host-side port mapping in `docker-compose.yml`:

```yaml
ports:
  - "3001:3000"   # Use 3001 on host instead of 3000
```
